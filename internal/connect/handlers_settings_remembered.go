package connect

import (
	"context"
	"os"
	"sort"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
)

type rememberedEntry struct {
	Key      string `json:"key"`
	ToolName string `json:"tool_name"`
	ArgsHash string `json:"args_hash"`
	Reason   string `json:"reason,omitempty"`
	Allowed  bool   `json:"allowed"`
}

type rememberedStoreData struct {
	Allowed map[string]struct {
		ToolName string `json:"tool_name"`
		ArgsHash string `json:"args_hash"`
	} `json:"allowed"`
	Rejected map[string]struct {
		ToolName string `json:"tool_name"`
		ArgsHash string `json:"args_hash"`
		Reason   string `json:"reason"`
	} `json:"rejected"`
}

func (c *Connector) handleRememberedList(_ context.Context, _ *Request) (any, error) {
	path, err := config.GetConfigPath("permissions.json")
	if err != nil {
		return map[string]any{"remembered": []rememberedEntry{}}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"remembered": []rememberedEntry{}}, nil
	}
	var d rememberedStoreData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "parse permissions: " + err.Error()}
	}
	var out []rememberedEntry
	for key, e := range d.Allowed {
		out = append(out, rememberedEntry{Key: key, ToolName: e.ToolName, ArgsHash: e.ArgsHash, Allowed: true})
	}
	for key, e := range d.Rejected {
		out = append(out, rememberedEntry{Key: key, ToolName: e.ToolName, ArgsHash: e.ArgsHash, Reason: e.Reason, Allowed: false})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ToolName != out[j].ToolName {
			return out[i].ToolName < out[j].ToolName
		}
		return out[i].Key < out[j].Key
	})
	return map[string]any{"remembered": out}, nil
}

type rememberedRevokeParams struct {
	Key string `json:"key"`
}

func (c *Connector) handleRememberedRevoke(_ context.Context, req *Request) (any, error) {
	var p rememberedRevokeParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.Key == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "key is required"}
	}
	path, err := config.GetConfigPath("permissions.json")
	if err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: "no remembered permissions"}
	}
	var d rememberedStoreData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "parse permissions: " + err.Error()}
	}
	delete(d.Allowed, p.Key)
	delete(d.Rejected, p.Key)
	out, merr := json.MarshalIndent(d, "", "  ")
	if merr != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: merr.Error()}
	}
	if werr := os.WriteFile(path, out, 0o600); werr != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: werr.Error()}
	}
	Audit("permissions.revoke", map[string]any{"key": p.Key})
	return map[string]any{"key": p.Key, "revoked": true}, nil
}
