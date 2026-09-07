package notify

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

type EmailNotificationService interface {
	Send(ctx context.Context, to, cc, bcc []string, subject, message string) error
	// SendOutreach is the same as Send but lets the caller override
	// the Reply-To header. Used by the admin outreach composer so a
	// noreply From: address can still funnel replies into a real
	// support inbox. Empty replyTo falls back to the From address.
	SendOutreach(ctx context.Context, to []string, replyTo, subject, message string) error
}

type emailNotificationService struct {
	Name    string
	Address string
	Client  *sesv2.Client
	From    string
}

func NewEmailNotficiationService(ctx context.Context, name, address string) (EmailNotificationService, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		errs.CaptureException(err)
		return nil, err
	}

	client := sesv2.NewFromConfig(cfg)
	return &emailNotificationService{
		Name:    name,
		Address: address,
		Client:  client,
	}, nil
}

func (s *emailNotificationService) Send(ctx context.Context, to, cc, bcc []string, subject, message string) error {
	from := fmt.Sprintf("%s <%s>", s.Name, s.Address)

	input := &sesv2.SendEmailInput{
		Destination: &types.Destination{
			ToAddresses:  to,
			CcAddresses:  cc,
			BccAddresses: bcc,
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Body: &types.Body{
					Html: &types.Content{
						Data: &message,
					},
				},
				Subject: &types.Content{
					Data: &subject,
				},
			},
		},
		FromEmailAddress: &from,
	}

	_, err := s.Client.SendEmail(ctx, input)
	if err != nil {
		errs.CaptureException(err)
		return err
	}

	return err
}

// SendOutreach is Send with an explicit Reply-To. SES exposes
// ReplyToAddresses on the SendEmailInput so we don't have to forge MIME
// headers ourselves.
func (s *emailNotificationService) SendOutreach(ctx context.Context, to []string, replyTo, subject, message string) error {
	from := fmt.Sprintf("%s <%s>", s.Name, s.Address)

	input := &sesv2.SendEmailInput{
		Destination: &types.Destination{
			ToAddresses: to,
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Body: &types.Body{
					Html: &types.Content{Data: &message},
				},
				Subject: &types.Content{Data: &subject},
			},
		},
		FromEmailAddress: &from,
	}
	if replyTo != "" {
		input.ReplyToAddresses = []string{replyTo}
	}

	_, err := s.Client.SendEmail(ctx, input)
	if err != nil {
		errs.CaptureException(err)
		return err
	}
	return nil
}
