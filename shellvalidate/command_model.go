package shellvalidate

type commandModel struct {
	names             []string
	formatArgument    int
	bracketTerminator bool
	mutatesShell      bool
	consumesStdin     bool
}

var commandModels = buildCommandModels()

func buildCommandModels() map[string]commandModel {
	models := []commandModel{
		{names: []string{".", "source"}, formatArgument: -1, mutatesShell: true},
		{names: []string{":", "break", "continue", "cd", "command", "builtin", "declare", "typeset", "local", "export", "readonly", "unset", "eval", "exec", "exit", "return", "getopts", "set", "shift", "shopt", "trap", "let", "wait", "jobs", "fg", "bg", "kill", "umask"}, formatArgument: -1, mutatesShell: true},
		{names: []string{"read", "mapfile", "readarray"}, formatArgument: -1, mutatesShell: true, consumesStdin: true},
		{names: []string{"printf"}, formatArgument: 0},
		{names: []string{"echo"}, formatArgument: -1},
		{names: []string{"test"}, formatArgument: -1},
		{names: []string{"["}, formatArgument: -1, bracketTerminator: true},
		{names: []string{"find", "xargs", "grep", "sed", "awk", "ssh", "sudo", "env", "rm", "cp", "mv", "ln", "mkdir", "tar"}, formatArgument: -1},
	}
	result := make(map[string]commandModel)
	for _, model := range models {
		for _, name := range model.names {
			result[name] = model
		}
	}
	return result
}
