package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
	"github.com/jaas/jaas/internal/organization/repositories"
	"gorm.io/gorm"
)

// OrgChartService builds the tenant forest, user chains, and headcounts. FR-O001..FR-O004.
type OrgChartService interface {
	GetChart(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, maxDepth int, includeInactive bool) (*dto.OrgChartResponse, error)
	GetUserChain(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.UserChainResponse, error)
}

// chartCache is the minimal cache surface the org chart needs (satisfied by RedisClient).
type chartCache interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key string, value string, expiration time.Duration) error
}

type orgChartService struct {
	deptRepo    repositories.DepartmentRepository
	teamRepo    repositories.TeamRepository
	mappingRepo repositories.MappingRepository
	desigRepo   repositories.DesignationRepository
	cache       chartCache
}

// NewOrgChartService creates an OrgChartService. Cache may be nil (compute direct).
func NewOrgChartService(
	deptRepo repositories.DepartmentRepository,
	teamRepo repositories.TeamRepository,
	mappingRepo repositories.MappingRepository,
	desigRepo repositories.DesignationRepository,
	cache chartCache,
) OrgChartService {
	return &orgChartService{deptRepo: deptRepo, teamRepo: teamRepo, mappingRepo: mappingRepo, desigRepo: desigRepo, cache: cache}
}

func chartCacheKey(tenantID uuid.UUID, maxDepth int, includeInactive bool) string {
	return fmt.Sprintf("org:chart:%s:%d:%t", tenantID.String(), maxDepth, includeInactive)
}

func (s *orgChartService) GetChart(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, maxDepth int, includeInactive bool) (*dto.OrgChartResponse, error) {
	if maxDepth <= 0 || maxDepth > 10 {
		maxDepth = 10
	}
	key := chartCacheKey(tenantID, maxDepth, includeInactive)
	if s.cache != nil {
		if raw, found, err := s.cache.Get(ctx, key); err == nil && found && raw != "" {
			var cached dto.OrgChartResponse
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return &cached, nil
			}
		}
	}

	roots, err := s.deptRepo.FindRoots(ctx, db, tenantID)
	if err != nil {
		return nil, err
	}
	nodes := make([]dto.OrgChartNode, 0, len(roots))
	for i := range roots {
		if roots[i].Status != "active" && !includeInactive {
			continue
		}
		node, err := s.buildNode(ctx, db, tenantID, &roots[i], 0, maxDepth, includeInactive)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *node)
	}
	resp := &dto.OrgChartResponse{Data: nodes}
	if s.cache != nil {
		if raw, err := json.Marshal(resp); err == nil {
			_ = s.cache.Set(ctx, key, string(raw), 60*time.Second)
		}
	}
	return resp, nil
}

func (s *orgChartService) buildNode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, dept *models.Department, depth, maxDepth int, includeInactive bool) (*dto.OrgChartNode, error) {
	headcount, err := s.mappingRepo.CountActiveByDepartment(ctx, db, tenantID, dept.ID)
	if err != nil {
		return nil, err
	}
	teams, _, err := s.teamRepo.FindAll(ctx, db, tenantID, &dept.ID, 1, 1000)
	if err != nil {
		return nil, err
	}
	teamDTOs := make([]dto.TeamResponse, 0, len(teams))
	for i := range teams {
		if teams[i].Status != "active" && !includeInactive {
			continue
		}
		count, err := s.mappingRepo.CountActiveByTeam(ctx, db, tenantID, teams[i].ID)
		if err != nil {
			return nil, err
		}
		teamDTOs = append(teamDTOs, dto.TeamResponse{
			ID: teams[i].ID.String(), TenantID: teams[i].TenantID.String(),
			DepartmentID: teams[i].DepartmentID.String(), Name: teams[i].Name,
			Code: teams[i].Code, Description: teams[i].Description,
			LeadUserID: uuidToString(teams[i].LeadUserID), Status: teams[i].Status,
			MemberCount: count, CreatedAt: teams[i].CreatedAt, UpdatedAt: teams[i].UpdatedAt,
		})
	}
	node := &dto.OrgChartNode{
		ID: dept.ID.String(), Name: dept.Name, Code: dept.Code,
		Status: dept.Status, Headcount: headcount, Teams: teamDTOs, Children: []dto.OrgChartNode{},
	}
	if depth+1 >= maxDepth {
		return node, nil
	}
	children, err := s.deptRepo.FindChildren(ctx, db, tenantID, dept.ID)
	if err != nil {
		return nil, err
	}
	for i := range children {
		if children[i].Status != "active" && !includeInactive {
			continue
		}
		child, err := s.buildNode(ctx, db, tenantID, &children[i], depth+1, maxDepth, includeInactive)
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, *child)
	}
	return node, nil
}

func (s *orgChartService) GetUserChain(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.UserChainResponse, error) {
	primary, err := s.mappingRepo.FindPrimaryByUser(ctx, db, tenantID, userID)
	if err != nil {
		return nil, err
	}
	resp := &dto.UserChainResponse{UserID: userID.String(), Ancestors: []dto.DepartmentResponse{}, DirectReports: []string{}}
	if primary == nil {
		return resp, nil
	}
	resp.Mapping = mapMapping(primary)
	if primary.TeamID != nil {
		team, err := s.teamRepo.FindByID(ctx, db, tenantID, *primary.TeamID)
		if err != nil {
			return nil, err
		}
		if team != nil {
			count, err := s.mappingRepo.CountActiveByTeam(ctx, db, tenantID, team.ID)
			if err != nil {
				return nil, err
			}
			resp.Team = &dto.TeamResponse{
				ID: team.ID.String(), TenantID: team.TenantID.String(),
				DepartmentID: team.DepartmentID.String(), Name: team.Name,
				Code: team.Code, Description: team.Description,
				LeadUserID: uuidToString(team.LeadUserID), Status: team.Status,
				MemberCount: count, CreatedAt: team.CreatedAt, UpdatedAt: team.UpdatedAt,
			}
		}
	}
	if primary.DepartmentID != nil {
		dept, err := s.deptRepo.FindByID(ctx, db, tenantID, *primary.DepartmentID)
		if err != nil {
			return nil, err
		}
		if dept != nil {
			depth, path, err := deptDepth(ctx, db, tenantID, s.deptRepo, dept)
			if err != nil {
				return nil, err
			}
			_ = depth
			resp.Department = &dto.DepartmentResponse{
				ID: dept.ID.String(), TenantID: dept.TenantID.String(), Name: dept.Name,
				Code: dept.Code, Description: dept.Description,
				ParentDepartmentID: uuidToString(dept.ParentDepartmentID), Status: dept.Status,
				Depth: depth, Path: path, CreatedAt: dept.CreatedAt, UpdatedAt: dept.UpdatedAt,
			}
			cur := dept.ParentDepartmentID
			for cur != nil {
				parent, err := s.deptRepo.FindByID(ctx, db, tenantID, *cur)
				if err != nil || parent == nil {
					return resp, err
				}
				pdepth, ppath, err := deptDepth(ctx, db, tenantID, s.deptRepo, parent)
				if err != nil {
					return nil, err
				}
				resp.Ancestors = append([]dto.DepartmentResponse{{
					ID: parent.ID.String(), TenantID: parent.TenantID.String(), Name: parent.Name,
					Code: parent.Code, Description: parent.Description,
					ParentDepartmentID: uuidToString(parent.ParentDepartmentID), Status: parent.Status,
					Depth: pdepth, Path: ppath, CreatedAt: parent.CreatedAt, UpdatedAt: parent.UpdatedAt,
				}}, resp.Ancestors...)
				cur = parent.ParentDepartmentID
			}
		}
	}
	reports, err := s.mappingRepo.FindReports(ctx, db, tenantID, userID)
	if err != nil {
		return nil, err
	}
	for i := range reports {
		resp.DirectReports = append(resp.DirectReports, reports[i].UserID.String())
	}
	if resp.DirectReports == nil {
		resp.DirectReports = []string{}
	}
	return resp, nil
}
