package domain

var allowedTransitions = map[State]map[State]bool{
	StateDraft: {StateBaselineLocked: true}, StateBaselineLocked: {StateSealing: true}, StateSealing: {StateHeld: true, StateVerification: true}, StateHeld: {StateSealing: true}, StateVerification: {StateHeld: true, StateReleased: true}, StateReleased: {StateArchived: true}, StateArchived: {},
}

func CanTransition(from, to State) bool { return allowedTransitions[from][to] }
func Transition(c *SealCase, to State) error {
	if c.State == StateArchived {
		return Frozen()
	}
	if c.State == to {
		return Gate("个案已经处于目标状态")
	}
	if !CanTransition(c.State, to) {
		return Gate("不允许从 " + string(c.State) + " 迁移到 " + string(to))
	}
	c.State = to
	return nil
}
