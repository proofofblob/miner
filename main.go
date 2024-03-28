package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/enescakir/emoji"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc/eip4844"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/holiman/uint256"
	blobutil "github.com/offchainlabs/nitro/util/blobs"
	log "github.com/sirupsen/logrus"
)

var (
	rpcUrl = "https://1rpc.io/eth"

	chainId = big.NewInt(1)

	rpcClient *ethclient.Client

	privateKey *ecdsa.PrivateKey

	sender common.Address

	contract = common.HexToAddress("0x3F97a968B848c711373B7Fbedb077041c9Ace145")

	mintCount uint64

	maxMintCount uint64

	userGasPrice *big.Int

	userGasTipCap *big.Int

	target = big.NewInt(0)

	userLimit = big.NewInt(5)

	totalSupply = big.NewInt(4844)

	hashCount uint64

	blobCh = make(chan *kzg4844.Blob, 3)
)

func monitor() {
	st := time.Now()
	ticker := time.NewTicker(time.Second * 20)
	msg := emoji.Emoji("")
	for i := 0; i < 10; i++ {
		msg += emoji.Hammer
	}
	for {
		select {
		case <-ticker.C:
			log.WithFields(log.Fields{
				"hash rate":    fmt.Sprintf("%.2f", float64(hashCount)/time.Since(st).Seconds()),
				"total hashes": atomic.LoadUint64(&hashCount),
				"running time": time.Since(st),
			}).Info(msg)
		}
	}
}

func loopTarget() {
	ticker := time.NewTicker(time.Second * 3)
	for {
		resp, err := rpcClient.CallContract(context.Background(), ethereum.CallMsg{
			To:   &contract,
			Data: common.Hex2Bytes("d4b83992"),
		}, nil)
		if err != nil {
			log.WithError(err).Error("failed to call contract")
			continue
		}
		nowTarget := big.NewInt(0).SetBytes(resp)
		if nowTarget.Cmp(target) > 0 {
			log.WithField("target", common.BytesToHash(nowTarget.Bytes()).Hex()).Info("target updated")
			target = nowTarget
		}
		<-ticker.C
	}
}

func checkMintLimit() (ok bool) {
	resp, err := rpcClient.CallContract(context.Background(), ethereum.CallMsg{
		To:   &contract,
		Data: common.Hex2Bytes("1aa5e872000000000000000000000000" + sender.Hex()[2:]), // userLimit
	}, nil)
	if err != nil {
		log.WithError(err).Error("failed to call contract")
		return
	}

	log.Infof("you have minted %d", big.NewInt(0).SetBytes(resp).Uint64())

	if big.NewInt(0).SetBytes(resp).Cmp(userLimit) >= 0 {
		return true
	}

	resp, err = rpcClient.CallContract(context.Background(), ethereum.CallMsg{
		To:   &contract,
		Data: common.Hex2Bytes("75794a3c"), // nextTokenId
	}, nil)
	if err != nil {
		log.WithError(err).Error("failed to call contract")
		return
	}

	log.Infof("total minted: %d", big.NewInt(0).SetBytes(resp).Uint64())

	if big.NewInt(0).SetBytes(resp).Cmp(totalSupply) > 0 {
		return true
	}
	return false
}

func initRPC() {
	var err error
	rpcClient, err = ethclient.Dial(rpcUrl)
	if err != nil {
		log.WithError(err).Fatal("failed to dial rpc")
		panic(err)
	}
}

func initTarget() {
	go loopTarget()
	ticker := time.NewTicker(time.Second * 1)
	for {
		select {
		case <-ticker.C:
			if target.Cmp(big.NewInt(0)) > 0 {
				return
			}
			log.Info("target not set yet")
		}
	}
}

func randomBlob() (b kzg4844.Blob, ok bool) {
	temp := make([]byte, 126*1024)
	rand.Read(temp)
	blobs, _ := blobutil.EncodeBlobs(temp)
	c, _ := kzg4844.BlobToCommitment(blobs[0])
	hashsum := kzg4844.CalcBlobHashV1(sha256.New(), &c)
	copy(hashsum[:], hashsum[1:])
	if big.NewInt(0).SetBytes(hashsum[:]).Cmp(target) < 0 {
		return blobs[0], true
	}
	return
}

func makeTx(b kzg4844.Blob) (signedTx *types.Transaction, err error) {
	commitment, _ := kzg4844.BlobToCommitment(b)
	proof, _ := kzg4844.ComputeBlobProof(b, commitment)
	sidecar := &types.BlobTxSidecar{
		Blobs:       []kzg4844.Blob{b},
		Commitments: []kzg4844.Commitment{commitment},
		Proofs:      []kzg4844.Proof{proof},
	}

	nonce, err := rpcClient.NonceAt(context.Background(), sender, nil)
	if err != nil {
		log.WithError(err).Error("failed to get nonce")
		return
	}

	log.WithFields(log.Fields{
		"nonce":   nonce,
		"address": sender,
	}).Info("rpc get nonce")

	parentHeader, err := rpcClient.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.WithError(err).Error("failed to get parent header")
		return
	}
	log.WithField("height", parentHeader.Number.String()).Info("rpc get parent header")
	parentExcessBlobGas := eip4844.CalcExcessBlobGas(*parentHeader.ExcessBlobGas, *parentHeader.BlobGasUsed)
	blobFeeCap := eip4844.CalcBlobFee(parentExcessBlobGas)
	maxBlobFeeCap := big.NewInt(0).Mul(blobFeeCap, big.NewInt(10)) // 10 times
	log.WithFields(log.Fields{
		"max blob fee cap(wei)": maxBlobFeeCap,
	}).Info("calc max blob fee cap")

	blobTx := &types.BlobTx{
		ChainID:    uint256.MustFromBig(chainId),
		Nonce:      nonce,
		GasTipCap:  uint256.MustFromBig(userGasTipCap),
		GasFeeCap:  uint256.MustFromBig(userGasPrice),
		To:         contract,
		Data:       common.Hex2Bytes("1249c58b"), // mint
		BlobFeeCap: uint256.MustFromBig(maxBlobFeeCap),
		BlobHashes: sidecar.BlobHashes(),
		Sidecar:    sidecar,
	}

	gasLimit, err := rpcClient.EstimateGas(context.Background(), ethereum.CallMsg{
		From:       sender,
		To:         &contract,
		Data:       blobTx.Data,
		BlobHashes: sidecar.BlobHashes(),
	})

	if err != nil {
		log.WithError(err).Error("failed to estimate gas")
		return
	}
	log.WithField("gas", gasLimit+20000).Info("rpc estimate gas")
	blobTx.Gas = gasLimit + 20000
	tx := types.NewTx(blobTx)

	signedTransaction, err := types.SignTx(tx, types.NewCancunSigner(chainId), privateKey)
	if err != nil {
		log.WithError(err).Error("failed to sign tx")
		return
	}
	return signedTransaction, nil
}

func sendTx(signedTx *types.Transaction) {
	err := rpcClient.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.WithError(err).Error("failed to send tx")
		return
	}
	log.WithField("hash", signedTx.Hash()).Info("tx sent")
	log.WithField("hash", signedTx.Hash()).Info("wait for transaction confirmation")

	for {
		time.Sleep(time.Second * 3)
		receipt, err := rpcClient.TransactionReceipt(context.Background(), signedTx.Hash())
		if err != nil {
			if err == ethereum.NotFound {
				log.WithField("hash", signedTx.Hash()).Info("wait for transaction confirmation")
				continue
			}
			log.WithError(err).Error("failed to get receipt")
			return
		}
		if receipt != nil {
			log.WithField("hash", signedTx.Hash()).Info("tx confirmed")
			break
		}
	}
	mintCount++
	if mintCount == maxMintCount {
		log.Info("mint count reached")
		os.Exit(0)
	}
}

func mineTask() {
	msg := emoji.Emoji("")
	for i := 0; i < 10; i++ {
		msg += emoji.Rocket
	}
	for {
		blob, ok := randomBlob()
		if ok {
			blobCh <- &blob
			log.Infof("%s   mined new blob", msg)
		}
		atomic.AddUint64(&hashCount, 1)
	}
}

func mine() {
	log.WithField("task count", runtime.NumCPU()-1).Info("create cpu task")
	go monitor()
	for i := 0; i < 1; i++ {
		go mineTask()
	}
	for blob := range blobCh {
		signedTx, err := makeTx(*blob)
		if err != nil {
			log.WithError(err).Error("failed to make tx")
			continue
		}
		if checkMintLimit() {
			log.Error("mint limit reached")
			os.Exit(0)
		}
		sendTx(signedTx)
	}
}

func main() {
	initRPC()
	if !cmd() {
		return
	}
	initTarget()
	mine()
}
