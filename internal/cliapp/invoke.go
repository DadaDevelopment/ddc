package cliapp

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func AgentInvoke(ctx context.Context, cfg Config, opts AgentOptions, out io.Writer) error {
	if strings.TrimSpace(opts.Text) == "" {
		return ConfigErrorf("invoke needs --text <message>")
	}
	if (opts.Project == "") != (opts.Env == "") {
		return ConfigErrorf("--project and --env go together")
	}
	name := opts.Agent
	if name == "" {
		spec, err := loadSpec(opts)
		if err != nil {
			return ConfigErrorf("pass --agent <name> or run inside the agent repo: %w", err)
		}
		name = spec.Name
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	client, err := agentClient(ctx, cfg, out)
	if err != nil {
		return err
	}
	if opts.Project != "" {
		ref, err := client.ResolveRef(ctx, opts.Project, opts.Env, "")
		if err != nil {
			return fmt.Errorf("resolve %s/%s: %w", opts.Project, opts.Env, err)
		}
		if _, err := client.AgentByName(ctx, ref.ProjectID, ref.EnvironmentID, name); err != nil {
			return err
		}
	}
	reply, err := client.SendAgentMessage(ctx, name, opts.Text)
	if err != nil {
		return fmt.Errorf("invoke %s: %w", name, err)
	}
	if strings.TrimSpace(reply) == "" {
		return fmt.Errorf("invoke %s: the agent answered with an empty reply", name)
	}
	fmt.Fprintln(out, reply)
	return nil
}
