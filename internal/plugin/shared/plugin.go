package shared

import (
	"net/rpc"

	"github.com/hashicorp/go-plugin"
)

var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "PEGG_PLUGIN",
	MagicCookieValue: "pegg",
}

var PluginMap = map[string]plugin.Plugin{
	"pegg": &PeggPluginRPC{},
}

type PeggPluginRPC struct {
	Impl Plugin
}

func (p *PeggPluginRPC) Server(*plugin.MuxBroker) (interface{}, error) {
	return &PeggPluginRPCServer{impl: p.Impl}, nil
}

func (p *PeggPluginRPC) Client(b *plugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &PeggPluginRPCClient{client: c}, nil
}

type PeggPluginRPCServer struct {
	impl Plugin
}

func (s *PeggPluginRPCServer) Name(args interface{}, resp *string) error {
	*resp = s.impl.Name()
	return nil
}

func (s *PeggPluginRPCServer) Version(args interface{}, resp *string) error {
	*resp = s.impl.Version()
	return nil
}

func (s *PeggPluginRPCServer) Description(args interface{}, resp *string) error {
	*resp = s.impl.Description()
	return nil
}

func (s *PeggPluginRPCServer) Boot(args *Deps, resp *interface{}) error {
	return s.impl.Boot(*args)
}

type PeggPluginRPCClient struct {
	client *rpc.Client
}

func (c *PeggPluginRPCClient) Name() string {
	var resp string
	err := c.client.Call("Plugin.Name", new(interface{}), &resp)
	if err != nil {
		return ""
	}
	return resp
}

func (c *PeggPluginRPCClient) Version() string {
	var resp string
	err := c.client.Call("Plugin.Version", new(interface{}), &resp)
	if err != nil {
		return ""
	}
	return resp
}

func (c *PeggPluginRPCClient) Description() string {
	var resp string
	err := c.client.Call("Plugin.Description", new(interface{}), &resp)
	if err != nil {
		return ""
	}
	return resp
}

func (c *PeggPluginRPCClient) Boot(deps Deps) error {
	var resp interface{}
	return c.client.Call("Plugin.Boot", &deps, &resp)
}

type PluginError struct {
	Message string
}

func (e *PluginError) Error() string {
	return e.Message
}
