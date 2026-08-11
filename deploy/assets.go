package deploy

import _ "embed"

// AgentInstaller is the installer served by the control plane so enrollment
// never depends on a matching repository checkout on the target host.
//
//go:embed install-agent.sh
var AgentInstaller []byte
