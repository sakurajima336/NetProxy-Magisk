package main

import (
	"flag"
	"strings"
)

// parseFlagsAnywhere 解析参数，允许 flag 出现在位置参数之后，返回位置参数。
//
// Go 标准 flag 包遇到第一个非 flag 参数就停止解析，
// 于是 `group set AI节点组 --providers x` 里的 --providers 会被忽略。
// 本函数先把参数重排成"全部 flag 在前"，再把位置参数单独返回，
// 使 `group set AI节点组 --providers x` 与 `group set --providers x AI节点组` 等价。
//
// 判定规则：以 - 开头的是 flag；不带 = 的 flag 会消费紧随其后的一个值。
func parseFlagsAnywhere(flags *flag.FlagSet, args []string) ([]string, error) {
	var flagArgs, positionals []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
			positionals = append(positionals, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		// 不带 = 的 flag 需要把下一个参数作为它的值。
		if !strings.Contains(arg, "=") && index+1 < len(args) {
			index++
			flagArgs = append(flagArgs, args[index])
		}
	}
	if err := flags.Parse(flagArgs); err != nil {
		return nil, err
	}
	return positionals, nil
}
