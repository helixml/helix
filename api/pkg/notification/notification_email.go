package notification

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/mail"
	"github.com/nikoksr/notify/service/mailgun"
	"github.com/rs/zerolog/log"
)

type Email struct {
	cfg     *config.Notifications
	enabled bool
}

func NewEmail(cfg *config.Notifications) (*Email, error) {
	e := &Email{
		cfg: cfg,
	}

	if cfg.Email.Mailgun.APIKey != "" {
		e.enabled = true
	}

	if cfg.Email.SMTP.Host != "" {
		e.enabled = true
	}

	return e, nil
}

func (e *Email) Enabled() bool {
	return e.enabled
}

func (e *Email) Notify(ctx context.Context, n *Notification) error {
	if n.Email == "" {
		// Nothing to do
		log.Ctx(ctx).Warn().Str("session_id", n.Session.ID).Msg("no email address provided for notification")
		return nil
	}

	client := e.getClient(n.Email)

	title, message, err := e.getEmailMessage(n)
	if err != nil {
		return err
	}

	err = client.Send(ctx, title, message)
	if err != nil {
		return fmt.Errorf("failed to send email to %s: %w", n.Email, err)
	}

	return nil
}

func (e *Email) getClient(email string) *notify.Notify {

	ntf := notify.New()

	if e.cfg.Email.Mailgun.APIKey != "" {
		log.Debug().Msg("using Mailgun")
		var opts []mailgun.Option
		if e.cfg.Email.Mailgun.Europe {
			opts = append(opts, mailgun.WithEurope())
		}

		mg := mailgun.New(e.cfg.Email.Mailgun.Domain, e.cfg.Email.Mailgun.APIKey, e.cfg.Email.SenderAddress, opts...)
		mg.AddReceivers(email)

		ntf.UseServices(mg)
	}

	if e.cfg.Email.SMTP.Host != "" {
		log.Debug().Msg("using SMTP")
		smtp := mail.New(e.cfg.Email.SenderAddress, e.cfg.Email.SMTP.Host+":"+e.cfg.Email.SMTP.Port)
		smtp.AuthenticateSMTP(e.cfg.Email.SMTP.Identity, e.cfg.Email.SMTP.Username, e.cfg.Email.SMTP.Password, e.cfg.Email.SMTP.Host)

		smtp.AddReceivers(email)

		ntf.UseServices(smtp)
	}
	return ntf
}

func (e *Email) getEmailMessage(n *Notification) (title, message string, err error) {
	switch n.Event {
	case EventCronTriggerComplete:
		var buf bytes.Buffer

		err = cronTriggerCompleteTmpl.Execute(&buf, &templateData{
			Message:     n.Message,
			SessionURL:  fmt.Sprintf("%s/session/%s", e.cfg.AppURL, n.Session.ID),
			SessionName: n.Session.Name,
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return n.Session.Name, buf.String(), nil
	case EventCronTriggerFailed:
		var buf bytes.Buffer

		err = cronTriggerFailedTmpl.Execute(&buf, &templateData{
			Message:     n.Message,
			SessionURL:  fmt.Sprintf("%s/session/%s", e.cfg.AppURL, n.Session.ID),
			SessionName: n.Session.Name,
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return n.Session.Name, buf.String(), nil
	default:
		return "", "", fmt.Errorf("unknown event '%s'", n.Event.String())
	}
}

type templateData struct {
	SessionURL  string
	Message     string
	SessionName string
}

var cronTriggerCompleteTmpl = template.Must(template.New("").Parse(cronTriggerCompleteTemplate))
var cronTriggerFailedTmpl = template.Must(template.New("").Parse(cronTriggerFailedTemplate))

const cronTriggerCompleteTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Helix Notification</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            margin: 0;
            padding: 0;
            background-color: #f5f5f5;
            color: #333;
        }
        .container {
            max-width: 600px;
            margin: 0 auto;
            background-color: white;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            overflow: hidden;
        }
        .header {
            background-color: #f8f9fa;
            padding: 30px 20px;
            text-align: center;
            border-bottom: 1px solid #e9ecef;
        }
        .logo {
            width: 40px;
            height: 40px;
            background-color: #000;
            border-radius: 50%;
            display: inline-block;
            margin-bottom: 20px;
            position: relative;
        }
        .logo::after {
            content: '';
            position: absolute;
            top: 8px;
            left: 8px;
            width: 24px;
            height: 24px;
            background-color: white;
            border-radius: 50%;
        }
        .logo::before {
            content: '';
            position: absolute;
            top: 12px;
            left: 12px;
            width: 16px;
            height: 16px;
            background-color: #000;
            border-radius: 50%;
        }
        .title {
            font-size: 24px;
            font-weight: 600;
            margin: 0 0 10px 0;
            color: #000;
        }
        .session-name {
            background-color: #e9ecef;
            color: #495057;
            padding: 8px 16px;
            border-radius: 20px;
            font-size: 14px;
            font-weight: 500;
            display: inline-block;
            margin-top: 10px;
        }
        .content {
            padding: 40px 30px;
            text-align: center;
        }
        .message {
            font-size: 16px;
            line-height: 1.6;
            color: #495057;
            margin-bottom: 30px;
        }
        .cta-button {
            display: inline-block;
            background-color: #000;
            color: white;
            padding: 12px 30px;
            text-decoration: none;
            border-radius: 6px;
            font-weight: 500;
            font-size: 16px;
            transition: background-color 0.2s;
        }
        .cta-button:hover {
            background-color: #333;
        }
        .footer {
            padding: 30px 20px;
            text-align: center;
            border-top: 1px solid #e9ecef;
        }
        .footer-logo {
            width: 30px;
            height: 30px;
            background-color: #000;
            border-radius: 50%;
            display: inline-block;
            margin-bottom: 15px;
            position: relative;
        }
        .footer-logo::after {
            content: '';
            position: absolute;
            top: 6px;
            left: 6px;
            width: 18px;
            height: 18px;
            background-color: white;
            border-radius: 50%;
        }
        .footer-logo::before {
            content: '';
            position: absolute;
            top: 9px;
            left: 9px;
            width: 12px;
            height: 12px;
            background-color: #000;
            border-radius: 50%;
        }
        .copyright {
            font-size: 12px;
            color: #6c757d;
            margin: 0;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="logo"></div>
            <h1 class="title">Your session is ready</h1>
            <div class="session-name">{{ .SessionName }}</div>
        </div>
        
        <div class="content">
            <div class="message">
                Your cron trigger has completed successfully. Click the button below to view your results in Helix.
            </div>
            
            <a href="{{ .SessionURL }}" class="cta-button" target="_blank">
                Continue reading
            </a>
        </div>
        
        <div class="footer">
            <div class="footer-logo"></div>
            <p class="copyright">© 2025 Helix AI LLC</p>
        </div>
    </div>
</body>
</html>
`

const cronTriggerFailedTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Helix Notification</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            margin: 0;
            padding: 0;
            background-color: #f5f5f5;
            color: #333;
        }
        .container {
            max-width: 600px;
            margin: 0 auto;
            background-color: white;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            overflow: hidden;
        }
        .header {
            background-color: #fff5f5;
            padding: 30px 20px;
            text-align: center;
            border-bottom: 1px solid #fed7d7;
        }
        .logo {
            width: 40px;
            height: 40px;
            background-color: #e53e3e;
            border-radius: 50%;
            display: inline-block;
            margin-bottom: 20px;
            position: relative;
        }
        .logo::after {
            content: '';
            position: absolute;
            top: 8px;
            left: 8px;
            width: 24px;
            height: 24px;
            background-color: white;
            border-radius: 50%;
        }
        .logo::before {
            content: '';
            position: absolute;
            top: 12px;
            left: 12px;
            width: 16px;
            height: 16px;
            background-color: #e53e3e;
            border-radius: 50%;
        }
        .title {
            font-size: 24px;
            font-weight: 600;
            margin: 0 0 10px 0;
            color: #c53030;
        }
        .session-name {
            background-color: #fed7d7;
            color: #c53030;
            padding: 8px 16px;
            border-radius: 20px;
            font-size: 14px;
            font-weight: 500;
            display: inline-block;
            margin-top: 10px;
        }
        .content {
            padding: 40px 30px;
            text-align: center;
        }
        .message {
            font-size: 16px;
            line-height: 1.6;
            color: #495057;
            margin-bottom: 30px;
        }
        .error-message {
            background-color: #fff5f5;
            border: 1px solid #fed7d7;
            border-radius: 6px;
            padding: 16px;
            margin: 20px 0;
            color: #c53030;
            font-family: 'Courier New', monospace;
            font-size: 14px;
            text-align: left;
            word-wrap: break-word;
        }
        .cta-button {
            display: inline-block;
            background-color: #e53e3e;
            color: white;
            padding: 12px 30px;
            text-decoration: none;
            border-radius: 6px;
            font-weight: 500;
            font-size: 16px;
            transition: background-color 0.2s;
        }
        .cta-button:hover {
            background-color: #c53030;
        }
        .footer {
            padding: 30px 20px;
            text-align: center;
            border-top: 1px solid #fed7d7;
        }
        .footer-logo {
            width: 30px;
            height: 30px;
            background-color: #e53e3e;
            border-radius: 50%;
            display: inline-block;
            margin-bottom: 15px;
            position: relative;
        }
        .footer-logo::after {
            content: '';
            position: absolute;
            top: 6px;
            left: 6px;
            width: 18px;
            height: 18px;
            background-color: white;
            border-radius: 50%;
        }
        .footer-logo::before {
            content: '';
            position: absolute;
            top: 9px;
            left: 9px;
            width: 12px;
            height: 12px;
            background-color: #e53e3e;
            border-radius: 50%;
        }
        .copyright {
            font-size: 12px;
            color: #6c757d;
            margin: 0;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="logo"></div>
            <h1 class="title">Session failed to complete</h1>
            <div class="session-name">{{ .SessionName }}</div>
        </div>
        
        <div class="content">
            <div class="message">
                Your cron trigger encountered an error and failed to complete successfully. You can view the details and retry in Helix.
            </div>
            
            {{ if .Message }}
            <div class="error-message">
                {{ .Message }}
            </div>
            {{ end }}
            
            <a href="{{ .SessionURL }}" class="cta-button" target="_blank">
                View details
            </a>
        </div>
        
        <div class="footer">
            <div class="footer-logo"></div>
            <p class="copyright">© 2025 Helix AI LLC</p>
        </div>
    </div>
</body>
</html>
`
