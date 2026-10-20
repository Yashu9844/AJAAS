package services

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
)

// Infrastructure failures surface as errors and roll the transaction back (NFR-D001).
func TestFailures_Propagate(t *testing.T) {
	ratings := models.Ratings{{Criterion: "Quality", Score: 3}, {Criterion: "Delivery", Score: 3}, {Criterion: "Collaboration", Score: 3}, {Criterion: "Ownership", Score: 3}}
	cases := []struct {
		fail string
		run  func(e *env, cy uuid.UUID, g *dto.GoalResponse) error
	}{
		{"cycles.create", func(e *env, _ uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.Create(ctx, e.admin(), dto.CreateCycleRequest{Title: "x", StartDate: "2026-01-01", EndDate: "2026-01-02"})
			return err
		}},
		{"cycles.find", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.Get(ctx, e.tenant, cy)
			return err
		}},
		{"cycles.find", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.NineBox(ctx, e.tenant, cy)
			return err
		}},
		{"cycles.list", func(e *env, _ uuid.UUID, _ *dto.GoalResponse) error {
			_, _, err := e.cycles.List(ctx, e.tenant, "", dto.ParsePage("", ""))
			return err
		}},
		{"cycles.update", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
			return err
		}},
		{"cycles.lock", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.Update(ctx, e.admin(), cy, dto.UpdateCycleRequest{})
			return err
		}},
		{"goals.open", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
			return err
		}},
		{"reviews.exists", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
			return err
		}},
		{"reviews.create", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
			return err
		}},
		{"managers", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
			return err
		}},
		{"employees.get", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
			return err
		}},
		{"outbox.create", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
			_ = err
			_, err = e.cycles.Complete(ctx, e.admin(), cy)
			return err
		}},
		{"goals.weightsum", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error { return e.newGoal(cy) }},
		{"goals.lockowner", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error { return e.newGoal(cy) }},
		{"goals.create", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error { return e.newGoal(cy) }},
		{"employees.byuser", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error { return e.newGoal(cy) }},
		{"goals.find", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error {
			_, err := e.goals.Get(ctx, e.as(e.alice), g.ID)
			return err
		}},
		{"goals.list", func(e *env, _ uuid.UUID, _ *dto.GoalResponse) error {
			_, _, err := e.goals.List(ctx, e.admin(), repositories.GoalFilter{}, dto.ParsePage("", ""))
			return err
		}},
		{"goals.lock", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error {
			_, err := e.goals.SetStatus(ctx, e.as(e.alice), g.ID, "completed")
			return err
		}},
		{"goals.update", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error {
			_, err := e.goals.SetStatus(ctx, e.as(e.alice), g.ID, "completed")
			return err
		}},
		{"goals.update", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error {
			_, err := e.goals.Update(ctx, e.as(e.alice), g.ID, dto.UpdateGoalRequest{Title: sp("x")})
			return err
		}},
		{"krs.create", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error { return e.newKR(g) }},
		{"krs.list", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error { return e.newKR(g) }},
		{"krs.list", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error {
			_, err := e.goals.Get(ctx, e.as(e.alice), g.ID)
			return err
		}},
		{"checkins.list", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error {
			_, err := e.goals.Get(ctx, e.as(e.alice), g.ID)
			return err
		}},
		{"checkins.create", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error {
			_, err := e.goals.CheckIn(ctx, e.as(e.alice), g.ID, dto.CheckInRequest{Progress: fxp("1")})
			return err
		}},
		{"krs.update", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error {
			if err := e.newKR(g); err != nil {
				return err
			}
			kr := e.anyKR(g.ID)
			_, err := e.goals.CheckIn(ctx, e.as(e.alice), g.ID, dto.CheckInRequest{KeyResultID: &kr, Value: fxp("1")})
			return err
		}},
		{"managers", func(e *env, _ uuid.UUID, g *dto.GoalResponse) error {
			_, err := e.goals.Get(ctx, e.as(e.carol), g.ID)
			return err
		}},
		{"reviews.list", func(e *env, _ uuid.UUID, _ *dto.GoalResponse) error {
			_, _, err := e.reviews.List(ctx, e.tenant, repositories.ReviewFilter{}, dto.ParsePage("", ""))
			return err
		}},
		{"reviews.list", func(e *env, _ uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.reviews.ListMine(ctx, e.as(e.alice), nil)
			return err
		}},
		{"reviews.list", func(e *env, _ uuid.UUID, _ *dto.GoalResponse) error {
			_, _, err := e.reviews.ListAssigned(ctx, e.as(e.alice), "", dto.ParsePage("", ""))
			return err
		}},
		{"reviews.find", func(e *env, _ uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.reviews.Get(ctx, e.admin(), uuid.New())
			return err
		}},
		{"reviews.lock", func(e *env, _ uuid.UUID, _ *dto.GoalResponse) error {
			_, err := e.reviews.Submit(ctx, e.as(e.alice), uuid.New(), dto.SubmitReviewRequest{Ratings: ratings})
			return err
		}},
		{"reviews.expire", func(e *env, cy uuid.UUID, _ *dto.GoalResponse) error {
			if _, err := e.cycles.LaunchReviews(ctx, e.admin(), cy); err != nil {
				return err
			}
			_, err := e.cycles.Complete(ctx, e.admin(), cy)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.fail, func(t *testing.T) {
			e := newEnv(t)
			cy := e.cycle("active")
			g := e.goal(e.as(e.alice), cy, e.alice, "10")
			e.st.fail[tc.fail] = errBoom
			e.ppl.fail[tc.fail] = errBoom
			if err := tc.run(e, cy, g); !errors.Is(err, errBoom) {
				t.Fatalf("%s: want boom, got %v", tc.fail, err)
			}
		})
	}
}

// Review submission failures after the review is found.
func TestFailures_Submit(t *testing.T) {
	for _, op := range []string{"reviews.update", "outbox.create", "cycles.lock"} {
		t.Run(op, func(t *testing.T) {
			e := newEnv(t)
			cy := e.cycle("active")
			e.goal(e.as(e.alice), cy, e.alice, "10")
			_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
			must(t, err)
			e.st.fail[op] = errBoom
			ratings := models.Ratings{{Criterion: "Quality", Score: 3}, {Criterion: "Delivery", Score: 3}, {Criterion: "Collaboration", Score: 3}, {Criterion: "Ownership", Score: 3}}
			_, err = e.reviews.Submit(ctx, e.as(e.alice), e.reviewOf(e.alice, models.ReviewSelf), dto.SubmitReviewRequest{Ratings: ratings})
			if !errors.Is(err, errBoom) {
				t.Fatalf("want boom, got %v", err)
			}
			if e.st.reviews[e.reviewOf(e.alice, models.ReviewSelf)].Status != models.ReviewPending {
				t.Fatal("rolled back")
			}
		})
	}
}

func (e *env) newGoal(cy uuid.UUID) error {
	_, err := e.goals.Create(ctx, e.as(e.alice), dto.CreateGoalRequest{CycleID: cy.String(), Title: "x", Weight: fx("1")})
	return err
}

func (e *env) newKR(g *dto.GoalResponse) error {
	_, err := e.goals.AddKeyResult(ctx, e.as(e.alice), g.ID, dto.KeyResultRequest{Title: "kr", StartValue: 0, TargetValue: fx("10")})
	return err
}

func (e *env) anyKR(goalID uuid.UUID) string {
	for id, k := range e.st.krs {
		if k.GoalID == goalID {
			return id.String()
		}
	}
	return ""
}
