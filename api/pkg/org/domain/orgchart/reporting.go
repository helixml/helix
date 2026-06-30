package orgchart

import "errors"

// ReportingLine is one edge of the org's reporting graph: ReportID
// reports to ManagerID. The relationship is many-to-many — a Bot may
// report to several managers and a manager may have many reports — so
// reporting lives in its own relation rather than as a column on the
// Bot. Deleting either endpoint Bot drops every line that references it
// (the store enforces this structurally).
//
// The graph is a DAG: cycles are rejected when a line is added (see the
// add-parent handler's ancestor walk).
type ReportingLine struct {
	OrgID     string
	ManagerID BotID
	ReportID  BotID
}

// NewReportingLine validates and constructs a ReportingLine. Both
// endpoints are required, must differ, and orgID must be set. A Bot
// cannot report to itself.
func NewReportingLine(orgID string, managerID, reportID BotID) (ReportingLine, error) {
	if orgID == "" {
		return ReportingLine{}, errors.New("reporting line orgID is empty")
	}
	if managerID == "" {
		return ReportingLine{}, errors.New("reporting line managerID is empty")
	}
	if reportID == "" {
		return ReportingLine{}, errors.New("reporting line reportID is empty")
	}
	if managerID == reportID {
		return ReportingLine{}, errors.New("bot cannot report to itself")
	}
	return ReportingLine{OrgID: orgID, ManagerID: managerID, ReportID: reportID}, nil
}
