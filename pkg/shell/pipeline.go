package shell

import (
	"context"
	"pcl/pkg/services"
)

// PipelineExecutor coordinates execution of pipelines and redirections.
type PipelineExecutor struct {
	proc services.ProcessService
}

func NewPipelineExecutor(proc services.ProcessService) *PipelineExecutor {
	if proc == nil {
		proc = services.NewDefaultProcessService(nil)
	}
	return &PipelineExecutor{proc: proc}
}

func (p *PipelineExecutor) Run(ctx context.Context, specs []services.CommandSpec, io services.IOService) (*services.ProcessResult, error) {
	return p.proc.ExecutePipeline(ctx, specs, io.Stdin(), io.Stdout(), io.Stderr())
}
