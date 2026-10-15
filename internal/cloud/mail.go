package cloud

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"net/url"
)

func (s *Server) sendAccountEmail(ctx context.Context, email, token, purpose string) error {
	if s.Config.MailFrom == "" {
		return fmt.Errorf("MAIL_FROM is not configured")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(s.Config.Region))
	if err != nil {
		return err
	}
	subject, action := "Verify your VideoFlow email", "verify-email"
	if purpose == "reset_password" {
		subject, action = "Reset your VideoFlow password", "reset-password"
	}
	link := s.Config.PublicOrigin + "/" + action + "?token=" + url.QueryEscape(token)
	client := sesv2.NewFromConfig(cfg)
	_, err = client.SendEmail(ctx, &sesv2.SendEmailInput{FromEmailAddress: aws.String(s.Config.MailFrom), Destination: &sestypes.Destination{ToAddresses: []string{email}}, Content: &sestypes.EmailContent{Simple: &sestypes.Message{Subject: &sestypes.Content{Data: aws.String(subject)}, Body: &sestypes.Body{Text: &sestypes.Content{Data: aws.String(subject + "\n\n" + link + "\n\nThis link expires in 30 minutes. If you did not request this, ignore this message.")}}}}})
	return err
}
