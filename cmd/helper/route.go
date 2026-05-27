package helper

import (
	"fmt"

	"github.com/fprl/simple-vps/internal/caddy"
	"github.com/fprl/simple-vps/internal/store"
	"github.com/fprl/simple-vps/internal/utils"
)

// Post-cutover, per-app routing lives in /etc/caddy/conf.d/*.caddy
// fragments written by `server app apply` from manifest routes.
// `generate-caddy` keeps writing the main /etc/caddy/Caddyfile (with
// the `import conf.d/*.caddy` line) so per-app fragments take effect.
// No client- or helper-side route CRUD remains.

type generateCaddyCmd struct {
	Force bool `help:"Force generation."`
}

func (c generateCaddyCmd) Run() error {
	routes, err := store.Default().ReadRoutes()
	if err != nil {
		utils.Die(err.Error(), 1)
	}

	changed, err := caddy.ApplyCaddyfile(routes, c.Force)
	if err != nil {
		utils.Die(err.Error(), 1)
	}

	if changed {
		fmt.Printf("Generated %s\n", caddy.CaddyfilePath())
	} else {
		fmt.Printf("%s already up to date\n", caddy.CaddyfilePath())
	}
	return nil
}
