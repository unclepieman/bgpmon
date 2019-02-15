package cmd

import (
	"fmt"
	pb "github.com/CSUNetSec/netsec-protobufs/bgpmon/v2"
	"github.com/spf13/cobra"
)

// closeCmd represents the close command
var closeCmd = &cobra.Command{
	Use:   "close SESS_ID",
	Short: "close a session ID",
	Long:  `closes a session associated with a SESS_ID that is provided.`,
	Args:  cobra.ExactArgs(1),
	Run:   closeSess,
}

func closeSess(cmd *cobra.Command, args []string) {
	sessId := args[0]

	if bc, clierr := NewBgpmonCli(bgpmondHost, bgpmondPort); clierr != nil {
		fmt.Printf("Error: %s\n", clierr)
	} else {
		defer bc.Close()
		emsg := &pb.CloseSessionRequest{
			SessionId: sessId,
		}
		ctx, cancel := getCtxWithCancel()
		defer cancel()
		if reply, err := bc.cli.CloseSession(ctx, emsg); err != nil {
			fmt.Printf("Error: %s\n", err)
		} else {
			fmt.Println("closed session with ID:", sessId, " server replied: ", reply)
		}
	}

}

func init() {
	rootCmd.AddCommand(closeCmd)
}