package connect

func (c *Connector) registerAllMethods() {
	c.registerMetaMethods()
	c.registerChatMethods()
	c.registerSessionMethods()
	c.registerSettingsMethods()
}
