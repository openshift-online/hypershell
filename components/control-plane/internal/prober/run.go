package prober

import "context"

// Run loads the prober configuration from the environment (OBS-PRB-08) and runs
// the probe loops and metrics endpoint until ctx is cancelled. It is the entry
// point for the control-plane `probe` subcommand (OBS-PRB-10) and never touches
// the reconciler, its watch streams, gateway database provisioning, or Helm
// gateway management.
func Run(ctx context.Context) error {
	return New(Load()).Run(ctx)
}
