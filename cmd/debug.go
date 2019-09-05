package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	pd "github.com/pingcap/pd/client"
	"github.com/pingcap/tidb-cdc/cdc"
	"github.com/pingcap/tidb-cdc/cdc/util"
	"github.com/pingcap/tidb/store/tikv/oracle"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func init() {
	rootCmd.AddCommand(pullCmd)

	pullCmd.Flags().StringVar(&pdAddr, "pd-addr", "localhost:2379", "address of PD")
}

var pdAddr string

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "pull kv change and print out",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		cli, err := pd.NewClient(strings.Split(pdAddr, ","), pd.SecurityOption{})
		if err != nil {
			fmt.Println(err)
			return
		}

		buf := cdc.MakeBuffer()
		ts := oracle.ComposeTS(time.Now().Unix()*1000, 0)
		detail := cdc.ChangeFeedDetail{}

		p := cdc.NewPuller(cli, ts, []util.Span{{nil, nil}}, detail, buf)

		g, ctx := errgroup.WithContext(context.Background())

		g.Go(func() error {
			return p.Run(ctx)
		})

		g.Go(func() error {
			for {
				entry, err := buf.Get(ctx)
				if err != nil {
					return err
				}

				fmt.Printf("%+v\n", entry.GetValue())
			}
		})

		err = g.Wait()

		if err != nil {
			fmt.Println(err)
		}
	},
}
