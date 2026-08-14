package shellvalidate

// maxDataflowIterations is a safety bound, not the expected convergence
// depth. The finite lattices below normally stabilize within a few joins.
const maxDataflowIterations = 32

type flowCertainty uint8

const (
	flowAbsent flowCertainty = iota
	flowMaybe
	flowDefinite
)

type optionValue uint8

const (
	optionOff optionValue = iota
	optionMaybe
	optionOn
)

type executionContext uint8

const (
	contextCurrentShell executionContext = iota
	contextSubshell
	contextPipeline
	contextFunction
	contextSource
	contextBackground
	contextCommandSubstitution
)

type flowState struct {
	variables map[string]flowCertainty
	nounset   optionValue
}

type flowFrame struct {
	kind executionContext
}

type dataflowAnalyzer struct {
	analysis *analysisContext
}

func newFlowState() flowState {
	state := flowState{variables: make(map[string]flowCertainty)}
	for name := range predefinedVariables() {
		state.variables[name] = flowDefinite
	}
	return state
}

func cloneFlowState(state flowState) flowState {
	result := flowState{variables: make(map[string]flowCertainty, len(state.variables)), nounset: state.nounset}
	for name, certainty := range state.variables {
		result.variables[name] = certainty
	}
	return result
}

func joinFlowStates(left, right flowState) flowState {
	result := flowState{variables: make(map[string]flowCertainty), nounset: joinOption(left.nounset, right.nounset)}
	for name, certainty := range left.variables {
		result.variables[name] = joinCertainty(certainty, right.variables[name])
	}
	for name, certainty := range right.variables {
		if _, exists := result.variables[name]; !exists {
			result.variables[name] = joinCertainty(left.variables[name], certainty)
		}
	}
	return result
}

func joinCertainty(left, right flowCertainty) flowCertainty {
	if left == right {
		return left
	}
	return flowMaybe
}

func joinOption(left, right optionValue) optionValue {
	if left == right {
		return left
	}
	return optionMaybe
}

func equalFlowStates(left, right flowState) bool {
	if left.nounset != right.nounset || len(left.variables) != len(right.variables) {
		return false
	}
	for name, certainty := range left.variables {
		if right.variables[name] != certainty {
			return false
		}
	}
	return true
}

func (flow *dataflowAnalyzer) analyze(nodes []Node, state flowState, frame flowFrame) flowState {
	for _, node := range nodes {
		state = flow.analyzeNode(node, state, frame)
	}
	return state
}

func (flow *dataflowAnalyzer) analyzeNode(node Node, state flowState, frame flowFrame) flowState {
	if node.incomplete {
		return state
	}
	switch node.kind {
	case NodeCommand, NodeAssignment:
		return flow.analyzeCommand(node, state, frame)
	case NodeList:
		return flow.analyzeConditionalList(node, state, frame)
	case NodePipeline:
		for _, child := range node.children {
			flow.analyzeNode(child, cloneFlowState(state), flowFrame{kind: contextPipeline})
		}
		return state
	case NodeSubshell, NodeCoprocess:
		flow.analyze(node.children, cloneFlowState(state), flowFrame{kind: contextSubshell})
		return state
	case NodeFunction:
		flow.analyze(node.children, cloneFlowState(state), flowFrame{kind: contextFunction})
		return state
	case NodeIf:
		return flow.analyzeIf(node, state, frame)
	case NodeWhile, NodeUntil, NodeFor:
		return flow.analyzeLoop(node, state, frame)
	case NodeCase:
		return flow.analyzeCase(node, state, frame)
	default:
		return flow.analyze(node.children, state, frame)
	}
}

func (flow *dataflowAnalyzer) analyzeConditionalList(node Node, state flowState, frame flowFrame) flowState {
	if len(node.children) == 0 {
		return state
	}
	left := flow.analyzeNode(node.children[0], state, frame)
	if len(node.children) == 1 {
		return left
	}
	truth := staticNodeTruth(node.children[0])
	if node.operator == "&&" && truth == truthFalse || node.operator == "||" && truth == truthTrue {
		return left
	}
	right := flow.analyzeNode(node.children[1], cloneFlowState(left), frame)
	if node.operator == "&&" && truth == truthTrue || node.operator == "||" && truth == truthFalse {
		return right
	}
	return joinFlowStates(left, right)
}

func (flow *dataflowAnalyzer) analyzeIf(node Node, state flowState, frame flowFrame) flowState {
	conditions, body, alternate := childrenByRole(node.children)
	afterCondition := flow.analyze(conditions, state, frame)
	switch staticSequenceTruth(conditions) {
	case truthTrue:
		return flow.analyze(body, afterCondition, frame)
	case truthFalse:
		return flow.analyze(alternate, afterCondition, frame)
	}
	primary := flow.analyze(body, cloneFlowState(afterCondition), frame)
	secondary := cloneFlowState(afterCondition)
	if len(alternate) != 0 {
		secondary = flow.analyze(alternate, secondary, frame)
	}
	return joinFlowStates(primary, secondary)
}

func (flow *dataflowAnalyzer) analyzeLoop(node Node, state flowState, frame flowFrame) flowState {
	conditions, body, _ := childrenByRole(node.children)
	afterCondition := flow.analyze(conditions, state, frame)
	if node.kind == NodeWhile && staticSequenceTruth(conditions) == truthFalse || node.kind == NodeUntil && staticSequenceTruth(conditions) == truthTrue {
		return afterCondition
	}
	entry := cloneFlowState(afterCondition)
	if node.kind == NodeFor && len(node.words) > 0 {
		name, known := staticWordValue(node.words[0])
		if known && validName(name) {
			entry.variables[name] = flowDefinite
		}
	}
	current := cloneFlowState(entry)
	for iteration := 0; iteration < maxDataflowIterations; iteration++ {
		bodyOut := flow.analyze(body, cloneFlowState(current), frame)
		next := joinFlowStates(entry, bodyOut)
		if equalFlowStates(next, current) {
			current = next
			break
		}
		current = next
		if iteration == maxDataflowIterations-1 {
			flow.analysis.inexact(node.span.Start.Offset, node.span.End.Offset, "control-flow analysis reached its fixed-point iteration limit")
		}
	}
	// while/until and implicit positional for-lists may execute zero times.
	if node.kind != NodeFor || len(node.words) <= 1 {
		return joinFlowStates(afterCondition, current)
	}
	return current
}

func (flow *dataflowAnalyzer) analyzeCase(node Node, state flowState, frame flowFrame) flowState {
	for _, word := range node.words {
		flow.checkWordReferences(word, state)
	}
	result := cloneFlowState(state) // no pattern is always possible.
	for _, arm := range node.children {
		armOut := flow.analyze(arm.children, cloneFlowState(state), frame)
		result = joinFlowStates(result, armOut)
	}
	return result
}

func (flow *dataflowAnalyzer) analyzeCommand(node Node, state flowState, frame flowFrame) flowState {
	view := commandViewFromNode(node)
	for _, assignment := range view.assignments {
		flow.checkWordReferences(assignment, state)
	}
	for _, argument := range view.arguments {
		flow.checkWordReferences(argument, state)
	}
	if node.kind == NodeAssignment {
		for _, assignment := range view.assignments {
			name, _, _ := assignmentValue(assignment)
			if name != "" {
				state.variables[name] = flowDefinite
			}
		}
	}
	if !view.hasName || !view.nameKnown {
		return state
	}
	if view.name == "set" {
		state.nounset = nounsetAfterSet(view.arguments, state.nounset)
	}
	if view.name != "read" {
		return state
	}
	for _, argument := range view.arguments {
		name, known := staticWordValue(argument)
		if known && validName(name) {
			state.variables[name] = flowDefinite
		}
	}
	if frame.kind == contextPipeline {
		flow.analysis.add("variables", flow.analysis.source.diagnostic("SHV1002", SeverityWarning, ConfidenceLikely, "read runs in a pipeline context, so assigned variables may not reach the parent shell", view.nameWord.span.Start.Offset, view.end))
	}
	return state
}

func (flow *dataflowAnalyzer) checkWordReferences(word Word, state flowState) {
	if state.nounset == optionOff {
		return
	}
	for _, reference := range parameterReferences(word) {
		if reference.name == "" || reference.name[0] >= '0' && reference.name[0] <= '9' {
			continue
		}
		certainty := state.variables[reference.name]
		if certainty == flowDefinite {
			continue
		}
		confidence := ConfidenceLikely
		message := "variable may be read while nounset can be enabled and no assignment is definite"
		if state.nounset == optionOn && certainty == flowAbsent {
			confidence = ConfidenceDefinite
			message = "variable is read while nounset is enabled and no assignment is visible"
		}
		flow.analysis.add("variables", flow.analysis.source.diagnostic("SHV1001", SeverityWarning, confidence, message, reference.start, reference.end))
	}
}

func nounsetAfterSet(arguments []Word, current optionValue) optionValue {
	result := current
	for index := 0; index < len(arguments); index++ {
		value, known := staticWordValue(arguments[index])
		if !known {
			result = optionMaybe
			continue
		}
		switch value {
		case "-u":
			result = optionOn
		case "+u":
			result = optionOff
		case "-o", "+o":
			if index+1 >= len(arguments) {
				continue
			}
			next, nextKnown := staticWordValue(arguments[index+1])
			if nextKnown && next == "nounset" {
				if value == "-o" {
					result = optionOn
				} else {
					result = optionOff
				}
				index++
			}
		}
	}
	return result
}

func childrenByRole(children []Node) (condition, body, alternate []Node) {
	for _, child := range children {
		switch child.role {
		case RoleCondition:
			condition = append(condition, child)
		case RoleAlternate:
			alternate = append(alternate, child)
		default:
			body = append(body, child)
		}
	}
	return condition, body, alternate
}

type truthValue uint8

const (
	truthUnknown truthValue = iota
	truthFalse
	truthTrue
)

func staticSequenceTruth(nodes []Node) truthValue {
	if len(nodes) == 0 {
		return truthUnknown
	}
	return staticNodeTruth(nodes[len(nodes)-1])
}

func staticNodeTruth(node Node) truthValue {
	if node.kind != NodeCommand {
		return truthUnknown
	}
	view := commandViewFromNode(node)
	if !view.hasName || !view.nameKnown || len(view.arguments) != 0 {
		return truthUnknown
	}
	switch view.name {
	case "true", ":":
		return truthTrue
	case "false":
		return truthFalse
	default:
		return truthUnknown
	}
}
