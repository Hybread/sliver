package notifications

import (
	"fmt"
	"strings"

	"github.com/bishopfox/sliver/server/core"
	"github.com/bishopfox/sliver/server/db/models"
)

func formatEvent(event core.Event) (string, string) {
	subject := fmt.Sprintf("Sliver event: %s", event.EventType)
	lines := []string{
		fmt.Sprintf("Event: %s", event.EventType),
	}

	if event.Session != nil {
		lines = append(lines, formatSession(event.Session)...)
	}
	if event.Pulse != nil {
		lines = append(lines, formatPulse(event.Pulse)...)
	}
	if event.Job != nil {
		lines = append(lines, formatJob(event.Job)...)
	}
	if event.Client != nil {
		lines = append(lines, formatClient(event.Client)...)
	}
	if event.Err != nil {
		lines = append(lines, fmt.Sprintf("Error: %s", event.Err.Error()))
	}

	return subject, strings.Join(lines, "\n")
}

func formatSession(session *core.Session) []string {
	lines := []string{
		fmt.Sprintf("Session ID: %s", session.ID),
	}
	if session.Name != "" {
		lines = append(lines, fmt.Sprintf("Name: %s", session.Name))
	}
	if session.Username != "" {
		lines = append(lines, fmt.Sprintf("User: %s", session.Username))
	}
	if session.Hostname != "" {
		lines = append(lines, fmt.Sprintf("Host: %s", session.Hostname))
	}
	if session.OS != "" {
		lines = append(lines, fmt.Sprintf("OS: %s", session.OS))
	}
	if session.Connection != nil {
		if session.Connection.Transport != "" {
			lines = append(lines, fmt.Sprintf("Transport: %s", session.Connection.Transport))
		}
		if session.Connection.RemoteAddress != "" {
			lines = append(lines, fmt.Sprintf("Remote: %s", session.Connection.RemoteAddress))
		}
	}
	return lines
}

func formatPulse(pulse *models.Pulse) []string {
	lines := []string{
		fmt.Sprintf("Pulse ID: %s", pulse.ID.String()),
	}
	if pulse.Name != "" {
		lines = append(lines, fmt.Sprintf("Name: %s", pulse.Name))
	}
	if pulse.Hostname != "" {
		lines = append(lines, fmt.Sprintf("Host: %s", pulse.Hostname))
	}
	if pulse.Username != "" {
		lines = append(lines, fmt.Sprintf("User: %s", pulse.Username))
	}
	if pulse.OS != "" {
		lines = append(lines, fmt.Sprintf("OS: %s", pulse.OS))
	}
	if pulse.Transport != "" {
		lines = append(lines, fmt.Sprintf("Transport: %s", pulse.Transport))
	}
	if pulse.RemoteAddress != "" {
		lines = append(lines, fmt.Sprintf("Remote: %s", pulse.RemoteAddress))
	}
	return lines
}

func formatJob(job *core.Job) []string {
	lines := []string{
		fmt.Sprintf("Job ID: %d", job.ID),
	}
	if job.Name != "" {
		lines = append(lines, fmt.Sprintf("Name: %s", job.Name))
	}
	if job.Protocol != "" {
		lines = append(lines, fmt.Sprintf("Protocol: %s", job.Protocol))
	}
	if job.Port != 0 {
		lines = append(lines, fmt.Sprintf("Port: %d", job.Port))
	}
	if len(job.Domains) > 0 {
		lines = append(lines, fmt.Sprintf("Domains: %s", strings.Join(job.Domains, ", ")))
	}
	return lines
}

func formatClient(client *core.Client) []string {
	lines := []string{
		fmt.Sprintf("Client ID: %d", client.ID),
	}
	if client.Operator != nil && client.Operator.Name != "" {
		lines = append(lines, fmt.Sprintf("Operator: %s", client.Operator.Name))
	}
	return lines
}
