package main

import (
	"bufio"
	"delsignbackend/wallets"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

const importKeyUsage = `usage: delsignbackend import-key -wallet <id> < keyfile

Reads a hex private key (with or without 0x) from the first line of stdin,
encrypts it with DELSIGN_KEY_ENC_KEY, and stores it as an address of the wallet.
The key is read from stdin rather than a flag so it doesn't show up in ps output;
keep it in a file rather than typing it on the command line.`

// runImportKey implements the import-key subcommand and returns the stored address.
func runImportKey(adb *wallets.AddressDB, args []string, stdin io.Reader) (string, error) {
	fs := flag.NewFlagSet("import-key", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(fs.Output(), importKeyUsage) }
	walletId := fs.Int("wallet", 0, "id of the wallet to add the key to")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if *walletId <= 0 || fs.NArg() > 0 {
		fs.Usage()
		return "", errors.New("import-key needs -wallet <id> and no other arguments")
	}

	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read key from stdin: %w", err)
	}
	key := strings.TrimSpace(line)
	if key == "" {
		return "", errors.New("no key on stdin")
	}

	return adb.ImportKey(*walletId, key)
}
