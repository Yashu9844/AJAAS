package services

import (
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/models"
	"github.com/jaas/jaas/internal/recruitment/validators"
)

func mapJob(j *models.Job) dto.JobResponse {
	return dto.JobResponse{ID: j.ID, Title: j.Title, DepartmentID: j.DepartmentID, DesignationID: j.DesignationID, Headcount: j.Headcount,
		HiredCount: j.HiredCount, Location: j.Location, EmploymentType: j.EmploymentType, MinExperienceYears: j.MinExperienceYears,
		Description: j.Description, Status: j.Status, OpenedAt: j.OpenedAt, ClosedAt: j.ClosedAt, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt}
}

func mapCandidate(c *models.Candidate) dto.CandidateResponse {
	return dto.CandidateResponse{ID: c.ID, JobID: c.JobID, FirstName: c.FirstName, LastName: c.LastName, Email: c.Email, Phone: c.Phone,
		Source: c.Source, ResumeURL: c.ResumeURL, ExpectedCTC: c.ExpectedCTC, NoticePeriodDays: c.NoticePeriodDays, Stage: c.Stage,
		HiredEmployeeID: c.HiredEmployeeID, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

func mapEvent(e *models.StageEvent) dto.StageEventResponse {
	return dto.StageEventResponse{FromStage: e.FromStage, ToStage: e.ToStage, ActorUserID: e.ActorUserID, Note: e.Note, CreatedAt: e.CreatedAt}
}

func mapInterview(i *models.Interview) dto.InterviewResponse {
	return dto.InterviewResponse{ID: i.ID, CandidateID: i.CandidateID, RoundName: i.RoundName, InterviewerEmployeeID: i.InterviewerEmployeeID,
		ScheduledAt: i.ScheduledAt, DurationMins: i.DurationMins, MeetingLink: i.MeetingLink, Status: i.Status, Rating: i.Rating,
		Recommendation: i.Recommendation, Feedback: i.Feedback, FeedbackAt: i.FeedbackAt, CreatedAt: i.CreatedAt}
}

func mapOffer(o *models.Offer) dto.OfferResponse {
	out := dto.OfferResponse{ID: o.ID, CandidateID: o.CandidateID, OfferedCTC: o.OfferedCTC, JoiningDate: o.JoiningDate.Format(validators.DateLayout),
		Status: o.Status, DecidedAt: o.DecidedAt, CreatedAt: o.CreatedAt}
	if o.ExpiresOn != nil {
		s := o.ExpiresOn.Format(validators.DateLayout)
		out.ExpiresOn = &s
	}
	return out
}

func mapList[M any, R any](rows []M, fn func(*M) R) []R {
	out := make([]R, len(rows))
	for i := range rows {
		out[i] = fn(&rows[i])
	}
	return out
}
