package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
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
		if count > 10 {
			return errors.New("max user mint count is 10")
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

	gasPrice, err := rpcClient.SuggestGasPrice(context.Background())
	if err != nil {
		fmt.Printf("rpcClient.SuggestGasPrice failed %v\n", err)
		return
	}

	fgas, _ := big.NewFloat(0).Quo(big.NewFloat(0).SetInt(gasPrice), big.NewFloat(1e9)).Float64()
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
				Help:    "max 10",
			},
			Validate: validate2,
		},
		{
			Name:     "gasPrice",
			Prompt:   &survey.Input{Message: "Input max gas price (gwei):", Default: fmt.Sprintf("%.2f", fgas)},
			Validate: validate3,
		},
		{
			Name:     "gasFeeTip",
			Prompt:   &survey.Input{Message: "Input gas tip cap (gwei):", Default: "0.01"},
			Validate: validate4,
		},
	}

	resp := struct {
		PrivateKey string
		MintCount  int
		GasPrice   float64
		GasFeeTip  float64
	}{}

	err = survey.Ask(qs, &resp)
	if err != nil {
		fmt.Printf("survey.Ask failed %v\n", err)
		return
	}
	ok = true
	return
}
