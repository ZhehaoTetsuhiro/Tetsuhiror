package main

import (
	"fmt"
	"os"

	"tetsuhiro/tthr/internal/tac"
	"tetsuhiro/tthr/internal/tcontainer"
)

// cmdKeygen: tthr keygen [选项]
func cmdKeygen(args []string) error {
	var opt struct {
		output string
		stdout bool
	}

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-o" || a == "--output":
			if i+1 >= len(args) {
				return fmt.Errorf("%s 需要参数", a)
			}
			i++
			opt.output = args[i]
		case a == "--stdout":
			opt.stdout = true
		default:
			return fmt.Errorf("未知选项: %s", a)
		}
	}

	priv, pub, err := tac.GenerateKey()
	if err != nil {
		return err
	}

	privText, err := tcontainer.FormatPrivateKey(priv)
	if err != nil {
		return err
	}
	pubText := fmt.Sprintf("-----BEGIN TTHR PUBLIC KEY-----\n%s\n-----END TTHR PUBLIC KEY-----\n", encodeB64(pub.MarshalPublicKey()))

	if opt.stdout {
		os.Stdout.Write(privText)
		fmt.Fprintln(os.Stderr, string(pubText))
		return nil
	}

	prefix := opt.output
	if prefix == "" {
		prefix = "tthr-key"
	}
	if err := os.WriteFile(prefix+".key", privText, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(prefix+".pub", []byte(pubText), 0o644); err != nil {
		return err
	}
	fmt.Printf("私钥: %s.key\n公钥: %s.pub\n", prefix, prefix)
	return nil
}

func encodeB64(b []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out []byte
	for i := 0; i < len(b); i += 3 {
		var n uint32
		rem := len(b) - i
		n = uint32(b[i]) << 16
		if rem > 1 {
			n |= uint32(b[i+1]) << 8
		}
		if rem > 2 {
			n |= uint32(b[i+2])
		}
		out = append(out, alphabet[n>>18&63], alphabet[n>>12&63])
		if rem > 1 {
			out = append(out, alphabet[n>>6&63])
		} else {
			out = append(out, '=')
		}
		if rem > 2 {
			out = append(out, alphabet[n&63])
		} else {
			out = append(out, '=')
		}
	}
	return string(out)
}
