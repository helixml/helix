package prompts

import "fmt"

func MCPConnectionUnavailableToolDescription(name string) string {
	return fmt.Sprintf("The %s connection is unavailable. Tell the user at the start of your response, continue unrelated work, and never claim dependent work completed. Call this to check the connection again before work that needs %s.", name, name)
}

func MCPConnectionAvailableToolDescription(name string) string {
	return fmt.Sprintf("Check the current %s connection. If its tools are missing from this coding agent, tell the user to reconnect the coding agent to load them.", name)
}

func MCPConnectionUnavailable(name string, authentication bool) string {
	reason := "failed"
	if authentication {
		reason = "did not accept its configured credentials"
	}
	return fmt.Sprintf("%s tools are unavailable because the connection %s. Configure or reconnect %s, then reconnect the coding agent to load its tools. You can continue with work that does not need %s.", name, reason, name, name)
}

func MCPConnectionRestored(name string) string {
	return fmt.Sprintf("%s is connected. Reconnect the coding agent if its tools are missing.", name)
}
