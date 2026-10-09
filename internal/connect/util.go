package connect

import (
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/utils/query"
)

func listQuery(page, size int) query.Query {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return query.Query{
		Page: query.Page{Number: page, Size: size},
		Sort: []query.Sort{{Column: "updated_at", Dir: query.Desc}},
		Filters: []query.Filter{
			{Column: "compaction_parent_id", Operator: query.OpEqual, Value: ""},
		},
	}
}

func maxConcurrency(cfg *config.ConnectConfig) int {
	if cfg == nil || cfg.MaxConcurrency <= 0 {
		return config.DefaultConnectConcurrency
	}
	return cfg.MaxConcurrency
}

func responseTimeout(cfg *config.ConnectConfig) time.Duration {
	if cfg == nil || cfg.ResponseTimeoutSecs == 0 {
		return time.Duration(config.DefaultConnectResponseTimeoutSecs) * time.Second
	}
	if cfg.ResponseTimeoutSecs < 0 {
		return 0
	}
	return time.Duration(cfg.ResponseTimeoutSecs) * time.Second
}

func availableProviders() []string { return llm.ListProviders() }

func queryFilterProject(workspace string) query.Filter {
	return query.Filter{Column: "project_dir", Operator: query.OpEqual, Value: workspace}
}
