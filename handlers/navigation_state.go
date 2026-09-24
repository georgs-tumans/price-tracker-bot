package handlers

// NavigationState holds the menu/conversation state of a single chat.
type NavigationState struct {
	CallbackMessageID    *int
	BackButtonEnabled    bool
	AwaitingUserInput    bool // A command has asked the user to type in a value
	CustomKeyboardActive bool // A custom reply keyboard is shown and must be removed after the input
	navigationStack      []*Command
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
