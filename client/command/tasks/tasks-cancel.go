package tasks

import (
	"context"

	"github.com/bishopfox/sliver/client/console"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/spf13/cobra"
)

// TasksCancelCmd - Cancel a pulse task before it's sent to the implant.
func TasksCancelCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	pulse := con.ActiveTarget.GetPulseInteractive()
	if pulse == nil {
		return
	}

	var idArg string
	if len(args) > 0 {
		idArg = args[0]
	}
	var task *clientpb.PulseTask
	var err error
	if idArg == "" {
		pulseTasks, err := con.Rpc.GetPulseTasks(context.Background(), &clientpb.Pulse{ID: pulse.ID})
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		tasks := []*clientpb.PulseTask{}
		for _, task := range pulseTasks.Tasks {
			if task.State == "pending" {
				tasks = append(tasks, task)
			}
		}
		if len(tasks) == 0 {
			con.PrintErrorf("No pending tasks for pulse\n")
			return
		}

		task, err = SelectPulseTask(tasks)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		con.Printf(console.UpN+console.Clearln, 1)
	} else {
		task, err = con.Rpc.GetPulseTaskContent(context.Background(), &clientpb.PulseTask{ID: idArg})
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		// Request and response content is not needed for cancelling a task
		task.Request = nil
		task.Response = nil
	}

	if task != nil {
		task, err := con.Rpc.CancelPulseTask(context.Background(), task)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		con.PrintInfof("Task %s canceled\n", task.ID)
	}
}
