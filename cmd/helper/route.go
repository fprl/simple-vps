package helper

import (
	"fmt"

	"github.com/fprl/simple-vps/internal/caddy"
	"github.com/fprl/simple-vps/internal/store"
	"github.com/fprl/simple-vps/internal/utils"
)

// Per ADR-0005 cutover item 27, the helper exposes only:
//
//   - `route list` — read-only inspection of the routes registered in
//     /etc/simple-vps/routes.json.
//
// The pre-cutover CRUD verbs (`proxy`, `static`, `redirect`, `remove`)
// are removed; per-app routes are now expressed in the manifest and
// applied by `server app apply` as a Caddyfile fragment under
// /etc/caddy/conf.d/. `generate-caddy` stays as the install-time
// bootstrap that writes the main /etc/caddy/Caddyfile (with the
// `import conf.d/*.caddy` line) so per-app fragments take effect.

type routeCmd struct {
	List routeListCmd `cmd:"" help:"List configured routes."`
}

type routeListCmd struct {
	JSON bool `name:"json" help:"Output JSON."`
}

func (c routeListCmd) Run() error {
	CmdRoutes(c.JSON)
	return nil
}

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
