package handler

import (
	"github.com/clawbake/clawbake/internal/auth"
	"github.com/clawbake/clawbake/internal/bot"
	"github.com/clawbake/clawbake/internal/config"
	"github.com/clawbake/clawbake/internal/database"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	DB        *database.Queries
	K8s       client.Client
	K8sConfig *rest.Config
	Auth      *auth.OIDCAuth
	DevAuth   *auth.DevAuth
	Config    *config.Config
	Bot       *bot.Bot
}
