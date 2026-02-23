A system for managing multiple openclaw instances in kubernetes.

This will consist of a multi user web application.  The admin user can setup
cross-instance defaults, adminster all existing instances (CRUD), etc.  The
web-app endpoint can also be used as a bot endpoint unless its better served in
a separate one.  Authenticate with OIDC IdP (Login with Google).  The web
application will run in the same kubernetes cluster it is provisoning the user's
openclaw instance in, and should be installed using helm.  Use postgresql for a
databse if needed.  Determine if defining a CRD+Operator is best for the
instance provisoning workflow over doing it all from the web application.

An openclaw instance will be provisioned for each unique user, with k8s
namespace isolation for each instance (clawbake ns for my app, clawbake-<user>
ns for each instance). Instances can be provisoned from the web application (one
per user for now) or from interaction with a bot (slack). That singular bot is
responsible for routing each user's messages to their own instance. Each
instance will maintain state in a persistent volume to ensure continuity for
that user.  Each instance should have a Ingress so that the user can use the web
to interact with the openclaw dashboard.

Use an agent team to develop this

You have control over the local development environment as we are running in a vscode devcontainer - use mise to install any tools needed, e.g. `mise use tmux`)
Use git to manage versions, performing checking at reasonable checkpoints, with major changes being performed on branches.
Make sure everything is well tested
Use a local kubernetes cluster (whichever works best, k3d, kind, etc) to aid in integration testing.

When creating implementation plans, write them to `docs/plans/<plan-name>.md`
Create and keep CLAUDE.md up-to-date
Prefer refactoring over backwards compatibility
Use latest versions of third-party packages
