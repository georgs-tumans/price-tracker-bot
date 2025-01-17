package handlers

type NavigationState struct {
	CallbackMessageID *int
	BackButtonEnabled bool
	navigationStack   []*Command
}

func (ns *NavigationState) Push(state *Command) {
	ns.navigationStack = append(ns.navigationStack, state)
}

func (ns *NavigationState) Pop() *Command {
	if len(ns.navigationStack) == 0 {
		return nil
	}

	last := ns.navigationStack[len(ns.navigationStack)-1]
	ns.navigationStack = ns.navigationStack[:len(ns.navigationStack)-1]

	return last
}

func (ns *NavigationState) Peek() *Command {
	if len(ns.navigationStack) == 0 {
		return nil
	}
	return ns.navigationStack[len(ns.navigationStack)-1]
}

func (ns *NavigationState) IsEmpty() bool {
	return len(ns.navigationStack) == 0
}
