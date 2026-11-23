package g10k

import (
	"os/exec"
)

// resourceTypesDirName is the name of the directory Puppet writes generated
// resource type metadata into, relative to an environment's root.
// See https://puppet.com/docs/puppet/latest/environment_isolation.html
const resourceTypesDirName = ".resource_types"

// generateTypesForEnvironment runs `puppet generate types` for the given
// environment, emulating r10k's deploy.generate_types setting:
// https://github.com/puppetlabs/r10k/blob/main/doc/dynamic-environments/configuration.mkd#generate_types
//
// basedir is the source's basedir (the parent of all its environment
// directories) and dirname is the environment's directory name inside it,
// matching the --environmentpath/--environment arguments r10k passes to
// puppet.
func (rt *Runtime) generateTypesForEnvironment(basedir string, dirname string) {
	if !rt.Config.GenerateTypes {
		return
	}
	if rt.DryRun || rt.PFMode {
		return
	}

	if _, err := exec.LookPath(rt.Config.PuppetPath); err != nil {
		rt.Warnf("WARNING: generate_types is enabled, but puppet_path '" + rt.Config.PuppetPath + "' is not executable, skipping type generation for environment " + dirname + ". Error: " + err.Error())
		return
	}

	cmd := rt.Config.PuppetPath + " generate types --environment " + dirname + " --environmentpath " + basedir + " --config " + rt.Config.PuppetConf
	rt.Debugf("Generating puppet types for environment '" + dirname + "'...")
	er := rt.executeCommand(cmd, "", rt.Config.Timeout, true, false)
	if er.returnCode != 0 {
		rt.Warnf("WARNING: failed to generate puppet types for environment " + dirname + ": " + er.output)
	}
}
