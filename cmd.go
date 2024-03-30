package main

import (
	"context"
	"fmt"
	"math/big"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ethereum/go-ethereum/consensus/misc/eip4844"
	"github.com/ethereum/go-ethereum/crypto"
)

func cmd() (ok bool) {
	validate := func(inp interface{}) error {
		input := inp.(string)
		if len(input) != 64 && len(input) != 66 {
			return fmt.Errorf("invalid length")
		}
		if len(input) == 66 {
			input = input[2:]
		}
		k, err := crypto.HexToECDSA(input)
		if err != nil {
			return err
		}
		privateKey = k
		sender = crypto.PubkeyToAddress(privateKey.PublicKey)
		return nil
	}

	validate2 := func(inp interface{}) error {
		input := inp.(string)
		if len(input) == 0 {
			return fmt.Errorf("invalid length")
		}
		count, err := strconv.Atoi(input)
		if err != nil {
			return err
		}
		if count > int(userLimit.Int64()) {
			return fmt.Errorf("max user mint count is %d", userLimit.Int64())
		}
		maxMintCount = uint64(count)
		return nil
	}

	validate3 := func(inp interface{}) error {
		input := inp.(string)
		if len(input) == 0 {
			return fmt.Errorf("invalid length")
		}

		fgwei, err := strconv.ParseFloat(input, 64)
		if err != nil {
			return err
		}

		userGasPrice = big.NewInt(int64(fgwei * 1e9))
		return nil
	}

	defaultGasPriceWei, err := rpcClient.SuggestGasPrice(context.Background())
	if err != nil {
		fmt.Printf("rpcClient.SuggestGasPrice failed %v\n", err)
		return
	}

	defaultGasPriceWei = big.NewInt(0).Add(defaultGasPriceWei, big.NewInt(1e10)) // gas cap base + 10gwei

	fgas, _ := big.NewFloat(0).Quo(big.NewFloat(0).SetInt(defaultGasPriceWei), big.NewFloat(1e9)).Float64()
	validate4 := func(inp interface{}) error {
		input := inp.(string)
		if len(input) == 0 {
			return fmt.Errorf("invalid length")
		}

		fgwei, err := strconv.ParseFloat(input, 64)
		if err != nil {
			return err
		}

		userGasTipCap = big.NewInt(int64(fgwei * 1e9))
		return nil
	}

	parentHeader, err := rpcClient.HeaderByNumber(context.Background(), nil)
	if err != nil {
		fmt.Printf("failed to get parent header, %v\n", err)
		return
	}
	parentExcessBlobGas := eip4844.CalcExcessBlobGas(*parentHeader.ExcessBlobGas, *parentHeader.BlobGasUsed)
	blobFeeCap := eip4844.CalcBlobFee(parentExcessBlobGas)
	defaultBlobFeeCap := big.NewInt(0).Mul(blobFeeCap, big.NewInt(2)) // 2 times
	defaultBlobFeeCapGwei, _ := big.NewFloat(0).Quo(big.NewFloat(0).SetInt(defaultBlobFeeCap), big.NewFloat(1e9)).Float64()

	if defaultBlobFeeCapGwei < 20 {
		defaultBlobFeeCapGwei = 20
	}

	validate5 := func(inp interface{}) error {
		input := inp.(string)
		if len(input) == 0 {
			return fmt.Errorf("invalid length")
		}

		fwei, err := strconv.ParseFloat(input, 64)
		if err != nil {
			return err
		}

		maxBlobFeeCap = big.NewInt(int64(fwei * 1e9))
		return nil
	}

	qs := []*survey.Question{
		{
			Name: "privateKey",
			Prompt: &survey.Password{
				Message: "Input your hex private key (In Windows you can try to paste with the right mouse button.):",
				Help:    "In Windows you can try to paste with the right mouse button.",
			},
			Validate: validate,
		},
		{
			Name: "mintCount",
			Prompt: &survey.Input{
				Message: "Input your mint count:",
				Help:    fmt.Sprintf("max mint count is %d", userLimit.Int64()),
			},
			Validate: validate2,
		},
		{
			Name: "gasPrice",
			Prompt: &survey.Input{
				Message: "Input max gas price (gwei):",
				Default: fmt.Sprintf("%.2f", fgas),
			},
			Validate: validate3,
		},
		{
			Name: "gasFeeTip",
			Prompt: &survey.Input{
				Message: "Input gas tip cap (gwei):",
				Default: "0.01",
			},
			Validate: validate4,
		},
		{
			Name: "maxBlobFeeCap",
			Prompt: &survey.Input{
				Message: "Input max blob gas fee cap (gwei):",
				Default: fmt.Sprintf("%.4f", defaultBlobFeeCapGwei),
			},
			Validate: validate5,
		},
	}

	resp := struct {
		PrivateKey    string
		MintCount     int
		GasPrice      float64
		GasFeeTip     float64
		MaxBlobFeeCap float64
	}{}

	err = survey.Ask(qs, &resp)
	if err != nil {
		return
	}
	ok = true
	return
}
