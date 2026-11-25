package api

import "github.com/myco-ai/myco/internal/domain"

// PageAttachment is a page-owned file snapshot; IDs are scoped by the page API.
type PageAttachment = domain.PageAttachment
type PageAttachmentUsage = domain.PageAttachmentUsage
type PageAttachmentMigrationReport = domain.PageAttachmentMigrationReport

type PageAttachmentList struct {
	Attachments       []*PageAttachment   `json:"attachments"`
	Usage             PageAttachmentUsage `json:"usage"`
	Next              string              `json:"next,omitempty"`
	AttachmentVersion int                 `json:"attachments_version"`
}
type PageAttachmentMigrationRequest struct {
	Project string `json:"project,omitempty"`
	DryRun  bool   `json:"dry_run"`
	Limit   int    `json:"limit,omitempty"`
}
