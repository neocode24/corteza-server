package rest

import (
	"context"
	"github.com/crusttech/crust/sam/rest/handlers"
	"github.com/crusttech/crust/sam/rest/request"
	"github.com/crusttech/crust/sam/service"
	"github.com/crusttech/crust/sam/types"
	"github.com/pkg/errors"
	"io"
	"time"
)

var _ = errors.Wrap

type (
	Attachment struct {
		svc service.AttachmentService
	}

	file struct {
		*types.Attachment
		content  io.ReadSeeker
		download bool
	}
)

func (Attachment) New(svc service.AttachmentService) *Attachment {
	return &Attachment{svc: svc}
}

func (ctrl *Attachment) Original(ctx context.Context, r *request.AttachmentOriginal) (interface{}, error) {
	return ctrl.get(r.AttachmentID, false, r.Download)

}

func (ctrl *Attachment) Preview(ctx context.Context, r *request.AttachmentPreview) (interface{}, error) {
	return ctrl.get(r.AttachmentID, true, false)
}

func (ctrl Attachment) get(ID uint64, preview, download bool) (handlers.Downloadable, error) {
	rval := &file{download: download}

	if att, err := ctrl.svc.FindByID(ID); err != nil {
		return nil, err
	} else {
		rval.Attachment = att
		if preview {
			rval.content, err = ctrl.svc.OpenPreview(att)
		} else {
			rval.content, err = ctrl.svc.OpenOriginal(att)
		}

		if err != nil {
			return nil, err
		}
	}

	return rval, nil
}

func (f *file) Download() bool {
	return f.download
}

func (f *file) Name() string {
	return f.Attachment.Name
}

func (f *file) ModTime() time.Time {
	return f.Attachment.CreatedAt
}

func (f *file) Content() io.ReadSeeker {
	return f.content
}