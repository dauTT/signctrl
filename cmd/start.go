package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/BlockscapeNetwork/signctrl/config"
	"github.com/BlockscapeNetwork/signctrl/privval"
	"github.com/BlockscapeNetwork/signctrl/types"
	"github.com/hashicorp/logutils"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tm_privval "github.com/tendermint/tendermint/privval"
)

var (
	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Starts the SignCTRL node",
		Run: func(cmd *cobra.Command, args []string) {
			// Load the config into memory.
			cfg, err := config.Load()
			if err != nil {
				fmt.Printf("couldn't load %v:\n%v", config.File, err)
				os.Exit(1)
			}
			cfgDir := config.Dir()

			// Set the logger and its mininum log level.
			logger := types.NewSyncLogger(os.Stderr, "", 0)
			filter := &logutils.LevelFilter{
				Levels:   types.LogLevels,
				MinLevel: logutils.LogLevel(cfg.Base.LogLevel),
				Writer:   os.Stderr,
			}
			logger.SetOutput(filter)

			// Load the state.
			state, err := config.LoadOrGenState(cfgDir)
			if err != nil {
				fmt.Printf("couldn't load %v (at %s):\n%v\n", config.StateFile, cfgDir, err)
				os.Exit(1)
			}

			// Initialize a new SCFilePV.
			pv := privval.NewSCFilePV(
				logger,
				cfg,
				state,
				tm_privval.LoadOrGenFilePV(
					privval.KeyFilePath(cfgDir),
					privval.StateFilePath(cfgDir),
				),
				&http.Server{Addr: fmt.Sprintf(":%v", privval.DefaultHTTPPort)},
			)
			pv.Gauges = types.RegisterGauges()

			// Start the SignCTRL service.
			if err := pv.Start(); err != nil {
				logger.Error(err.Error())
				if err := pv.Stop(); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				os.Exit(1)
			}

			// Wait either for the service itself or a system call to quit the process.
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-pv.Quit(): // Used for self-induced shutdown
				pv.Logger.Info("Shutting SignCTRL down... \u23FB (quit)")
			case <-sigs: // The sigs channel is only used for OS interrupt signals
				pv.Logger.Info("Shutting SignCTRL down... \u23FB (user/os interrupt)")
				if err := pv.Stop(); err != nil {
					logger.Error(err.Error())
					os.Exit(1)
				}
			}

			// Wait for all log messages to be printed out.
			time.Sleep(500 * time.Millisecond)

			// Terminate the process gracefully with exit code 0.
			os.Exit(0)
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.AddCommand(startCmd)
}

func initConfig() {
	cfgParts := strings.Split(config.File, ".")

	viper.SetConfigName(cfgParts[0])
	viper.SetConfigType(cfgParts[1])

	viper.AddConfigPath("$SIGNCTRL_CONFIG_DIR")
	viper.AddConfigPath("$HOME/.signctrl")
	viper.AddConfigPath(".")
}
