package domain

type ApprovalGate struct {
	URL      string
	Reason   string
	Risk     string
	Policy   string
	ChangeID ChangeID
	Phase    PhaseType
	TraceID  string
}
