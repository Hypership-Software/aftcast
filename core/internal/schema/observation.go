package schema

const ObservationVersion = 3

type Operation string

const (
	OperationRead    Operation = "read"
	OperationSearch  Operation = "search"
	OperationAsk     Operation = "ask"
	OperationPlan    Operation = "plan"
	OperationEdit    Operation = "edit"
	OperationTest    Operation = "test"
	OperationLint    Operation = "lint"
	OperationFormat  Operation = "format"
	OperationInspect Operation = "inspect"
	OperationExecute Operation = "execute"
	OperationSkill   Operation = "skill"
	OperationAgent   Operation = "agent"
	OperationFetch   Operation = "fetch"
	OperationOther   Operation = "other"
)

type ChangeStats struct {
	LinesAdded   int `json:"lines_added"`
	LinesRemoved int `json:"lines_removed"`
}
