package bot

import "github.com/labstack/echo/v4"

// RegisterRoutes mounts the bot's HTTP handlers onto an Echo group.
func (b *Bot) RegisterRoutes(g *echo.Group) {
	g.Use(b.verifySignature)
	g.POST("/events", b.HandleEvents)
	g.POST("/commands", b.HandleCommands)
}
