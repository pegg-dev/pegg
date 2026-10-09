package tools

import (
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/llm"
)

var reg = tool.NewRegistry()

func Register(t tool.Tool) error {
	return reg.Register(t)
}

func Unregister(name string) bool {
	return reg.Unregister(name)
}

func Get(name string) (tool.Tool, bool) {
	return reg.Get(name)
}

func List() []tool.Tool {
	return reg.List()
}

func Names() []string {
	return reg.Names()
}

func LLMTools() []llm.Tool {
	return reg.LLMTools()
}
