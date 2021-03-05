package privval

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/BlockscapeNetwork/signctrl/config"
	"github.com/BlockscapeNetwork/signctrl/connection"
	"github.com/BlockscapeNetwork/signctrl/types"
	tm_protoio "github.com/tendermint/tendermint/libs/protoio"
	tm_privval "github.com/tendermint/tendermint/privval"
	tm_privvalproto "github.com/tendermint/tendermint/proto/tendermint/privval"
)

const (
	// KeyFile is Tendermint's default file name for the private validator's keys.
	KeyFile = "priv_validator_key.json"

	// StateFile is Tendermint's default file name for the private validator's state.
	StateFile = "priv_validator_state.json"

	// maxRemoteSignerMsgSize determines the maximum size in bytes for the delimited
	// reader.
	maxRemoteSignerMsgSize = 1024 * 10

	// retryDialTimeout determines the default time in seconds SignCTRL waits for
	// a message from the validator until it assumes it has lost connection and
	// retries dialing it.
	retryDialTimeout = 15
)

// SCFilePV must implement the SignCtrled interface.
var _ types.SignCtrled = new(SCFilePV)

// SCFilePV is a wrapper for tm_privval.FilePV.
// Implements the SignCtrled interface by embedding BaseSignCtrled.
// Implements the Service interface by embedding BaseService.
type SCFilePV struct {
	types.BaseService
	types.BaseSignCtrled

	Logger     *log.Logger
	Config     *config.Config
	TMFilePV   *tm_privval.FilePV
	SecretConn net.Conn
}

// KeyFilePath returns the absolute path to the priv_validator_key.json file.
func KeyFilePath(cfgDir string) string {
	return cfgDir + "/" + KeyFile
}

// StateFilePath returns the absolute path to the priv_validator_state.json file.
func StateFilePath(cfgDir string) string {
	return cfgDir + "/" + StateFile
}

// NewSCFilePV creates a new instance of SCFilePV.
func NewSCFilePV(logger *log.Logger, cfg *config.Config, tmpv *tm_privval.FilePV) *SCFilePV {
	pv := &SCFilePV{
		Logger:   logger,
		Config:   cfg,
		TMFilePV: tmpv,
	}
	pv.BaseService = *types.NewBaseService(
		logger,
		"SignCTRL",
		pv,
	)
	pv.BaseSignCtrled = *types.NewBaseSignCtrled(
		logger,
		pv.Config.Base.Threshold,
		pv.Config.Base.StartRank,
		pv,
	)

	return pv
}

// run runs the main loop of SignCTRL. It handles incoming messages from the validator.
// In order to stop the goroutine, Stop() can be called outside of run(). The goroutine
// returns on its own once SignCTRL is forced to shut down.
func (pv *SCFilePV) run() {
	timeout := time.NewTimer(retryDialTimeout * time.Second)
	ctx, cancel := context.WithCancel(context.Background())

	for {
		select {
		case <-pv.Quit():
			pv.Logger.Printf("[DEBUG] signctrl: Terminating run goroutine: service stopped")
			cancel()
			// Note: Don't use pv.Stop() in here as it closes the pv.Quit() channel.
			return

		case <-timeout.C:
			pv.Logger.Printf("[INFO] signctrl: Lost connection to the validator... (no message for %v seconds)\n", retryDialTimeout)
			pv.SecretConn.Close()

			// Load the connection key from the config directory.
			connKey, err := connection.LoadConnKey(config.Dir())
			if err != nil {
				pv.Logger.Printf("[ERR] signctrl: couldn't load conn.key: %v", err)
				cancel()
				pv.Stop()
				return
			}

			// Dial the validator.
			pv.SecretConn, err = connection.RetrySecretDialTCP(
				pv.Config.Base.ValidatorListenAddress,
				connKey,
				pv.Logger,
			)
			if err != nil {
				pv.Logger.Printf("[ERR] signctrl: couldn't dial validator: %v", err)
				cancel()
				// Note: Don't use pv.Stop() in here, as RetrySecretDialTCP can only be stopped via SIGINT/SIGTERM.
				return
			}

		default:
			var msg tm_privvalproto.Message
			r := tm_protoio.NewDelimitedReader(pv.SecretConn, maxRemoteSignerMsgSize)
			if _, err := r.ReadMsg(&msg); err != nil {
				if err != io.EOF {
					pv.Logger.Printf("[ERR] signctrl: couldn't read message: %v\n", err)
				}
				continue
			}

			timeout.Reset(retryDialTimeout * time.Second)
			cancel()

			ctx, cancel = context.WithCancel(context.Background())
			resp, err := HandleRequest(ctx, &msg, pv)
			w := tm_protoio.NewDelimitedWriter(pv.SecretConn)
			if _, err := w.WriteMsg(resp); err != nil {
				pv.Logger.Printf("[ERR] signctrl: couldn't write message: %v\n", err)
			}
			if err != nil {
				pv.Logger.Printf("[ERR] signctrl: couldn't handle request: %v\n", err)
				if err == types.ErrMustShutdown {
					pv.Logger.Printf("[DEBUG] signctrl: Terminating run goroutine: %v\n", err)
					cancel()
					pv.Stop()
					pv.SecretConn.Close()
					return
				}
			}
		}
	}
}

// OnStart starts the main loop of the SignCtrled PrivValidator.
// Implements the Service interface.
func (pv *SCFilePV) OnStart() (err error) {
	pv.Logger.Printf("[INFO] signctrl: Starting SignCTRL on rank %v...\n", pv.GetRank())

	// Load the connection key from the config directory.
	connKey, err := connection.LoadConnKey(config.Dir())
	if err != nil {
		return fmt.Errorf("[ERR] signctrl: couldn't load conn.key: %v", err)
	}

	// Dial the validator.
	pv.SecretConn, err = connection.RetrySecretDialTCP(
		pv.Config.Base.ValidatorListenAddress,
		connKey,
		pv.Logger,
	)
	if err != nil {
		return fmt.Errorf("[ERR] signctrl: couldn't dial validator: %v", err)
	}

	// Run the main loop.
	go pv.run()

	return nil
}

// OnStop terminates the main loop of the SignCtrled PrivValidator.
// Implements the Service interface.
func (pv *SCFilePV) OnStop() {
	pv.Logger.Printf("[INFO] signctrl: Stopping SignCTRL on rank %v...\n", pv.GetRank())

	// Save rank to last_rank.json file if the shutdown was not self-induced.
	if err := pv.Save(config.Dir(), pv.Logger); err != nil {
		fmt.Printf("[ERR] signctrl: Couldn't save rank to %v: %v", LastRankFile, err)
		os.Exit(1)
	}
}
