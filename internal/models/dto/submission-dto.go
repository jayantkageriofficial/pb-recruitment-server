package dto

import "app/internal/models"

type SubmitSubmissionRequest struct {
	ContestID string                `json:"contest_id" validate:"required"`
	ProblemID string                `json:"problem_id" validate:"required"`
	Language  string                `json:"language" validate:"required_if=Type code"`
	Code      string                `json:"code" validate:"required_if=Type code"`
	Option    []int                 `json:"option" validate:"required_if=Type mcq,omitempty,min=1,dive,gte=0,lte=2147483647"`
	Type      models.SubmissionType `json:"type" validate:"required,oneof=mcq code"`
}

type SubmitSubmissionResponse struct {
	SubmissionID string `json:"submission_id"`
}

type RunCodeRequest struct {
	ContestID string `json:"contest_id" validate:"required"`
	ProblemID string `json:"problem_id" validate:"required"`
	Language  string `json:"language" validate:"required"`
	Code      string `json:"code" validate:"required,max=100000"`
	Stdin     string `json:"stdin" validate:"max=100000"`
}

type RunCodeResponse struct {
	Status        string `json:"status"`
	Stdout        string `json:"stdout,omitempty"`
	Stderr        string `json:"stderr,omitempty"`
	CompileOutput string `json:"compile_output,omitempty"`
	Message       string `json:"message,omitempty"`
	Time          string `json:"time,omitempty"`
	Memory        int    `json:"memory,omitempty"`
}

type ListProblemSubmissionsRequest struct {
	ProblemID string `query:"problem_id" validate:"required"`
	Page      int    `query:"page" validate:"min=0"`
}

type ListProblemSubmissionsResponse struct {
	Submissions []models.Submission `json:"submissions"`
}

type GetSubmissionDetailsResponse struct {
	models.Submission
	Code string `json:"code"`
}
