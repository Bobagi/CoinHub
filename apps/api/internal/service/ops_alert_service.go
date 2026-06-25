package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"coin-hub/internal/email"
)

// adminEmailLister returns the addresses to page about infrastructure problems.
type adminEmailLister interface {
	ListAdminEmails(loadContext context.Context) ([]string, error)
}

// OpsAlertService pages operators (admin users) about infrastructure problems that users can't fix —
// today, a stalled automation worker. It is best-effort: a no-op email transport (SMTP unset) just
// logs, and any send error is swallowed so alerting never affects the running app.
type OpsAlertService struct {
	adminLister adminEmailLister
	emailSender email.Sender
	appBaseURL  string
}

func NewOpsAlertService(adminLister adminEmailLister, emailSender email.Sender, appBaseURL string) *OpsAlertService {
	return &OpsAlertService{adminLister: adminLister, emailSender: emailSender, appBaseURL: appBaseURL}
}

// AlertWorkerStalled emails every admin that the trading automation worker has stopped checking in, so
// the operator can investigate before robots miss buys/sells for long.
func (service *OpsAlertService) AlertWorkerStalled(alertContext context.Context, staleFor time.Duration) {
	if service == nil || service.emailSender == nil || !service.emailSender.Enabled() {
		return
	}

	listContext, cancel := context.WithTimeout(alertContext, 5*time.Second)
	defer cancel()
	adminEmails, listError := service.adminLister.ListAdminEmails(listContext)
	if listError != nil {
		log.Printf("ops alert: could not list admin emails: %v", listError)
		return
	}

	staleText := staleFor.Round(time.Second).String()
	subject := "[CoinHub] Automation worker stalled — robots may not be trading"
	textBody := fmt.Sprintf(
		"The CoinHub automation worker has not recorded a heartbeat for %s.\n\n"+
			"While it is stalled, robots may not run their daily buys, take-profit reconciliation or "+
			"stop-loss. Please check the API logs:\n\n"+
			"  docker logs --since 30m coin-hub-api-1\n\n"+
			"Status: %s/api/v1/system/status   Liveness probe: %s/health/worker\n\n"+
			"This is an automated operations alert from %s.",
		staleText, service.appBaseURL, service.appBaseURL, service.appBaseURL,
	)
	htmlBody := fmt.Sprintf(
		"<p>The CoinHub automation worker has not recorded a heartbeat for <strong>%s</strong>.</p>"+
			"<p>While it is stalled, robots may not run their daily buys, take-profit reconciliation or "+
			"stop-loss. Please check the API logs:</p>"+
			"<pre>docker logs --since 30m coin-hub-api-1</pre>"+
			"<p>Liveness probe: <a href=\"%s/health/worker\">%s/health/worker</a></p>"+
			"<p style=\"color:#888\">Automated operations alert from %s.</p>",
		staleText, service.appBaseURL, service.appBaseURL, service.appBaseURL,
	)

	for _, adminEmail := range adminEmails {
		sendContext, sendCancel := context.WithTimeout(alertContext, 15*time.Second)
		if sendError := service.emailSender.Send(sendContext, email.Message{To: adminEmail, Subject: subject, TextBody: textBody, HTMLBody: htmlBody}); sendError != nil {
			log.Printf("ops alert: could not email admin %s: %v", adminEmail, sendError)
		}
		sendCancel()
	}
}
