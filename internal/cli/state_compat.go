package cli

import "vmlite/internal/state"

// Compatibility aliases while the root package is split into internal
// packages. New code should use internal/state directly.
type State = state.State
type ProvisionRecord = state.ProvisionRecord
type projectLayout = state.ProjectLayout
type projectLock = state.Lock

const (
	installPhaseInstalling = state.PhaseInstalling
	installPhaseInstalled  = state.PhaseInstalled
)

func newProjectLayout(configPath, stateDirFlag, name string) (projectLayout, error) {
	return state.NewProjectLayout(configPath, stateDirFlag, name)
}

func loadState(path string) (*State, error) {
	return state.Load(path)
}

func saveState(path string, st *State) error {
	return state.Save(path, st)
}

func acquireProjectLock(layout projectLayout) (*projectLock, error) {
	return state.AcquireLock(layout)
}
