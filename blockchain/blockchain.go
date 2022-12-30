package blockchain

import (
	"encoding/hex"
	"fmt"
	"os"
	"runtime"

	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks"

	// This can be used to verify that the blockchain exists
	dbFile = "./tmp/blocks/MANIFEST"

	// This is arbitrary data for our genesis block
	genesisData = "First Transaction from Genesis"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

func InitBlockChain(address string) *BlockChain {
	var lastHash []byte

	if DBexists(dbFile) {
		fmt.Println("blockchain already exists")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions(dbPath)
	db, err := badger.open(opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {

		// bad encapsulation, no reason to create the object in the transaction, db update logic could be shared by init and continue if this was split
		cbtx := CoinbaseTx(address, genesisData)
		genesis := Genesis(cbtx)
		fmt.Println("Genesis Created")
		err = txn.Set(genesis.Hash, genesis.Serialize())
		Handle(err)
		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err

	})
	return &BlockChain{lastHash, db}
}

func ContinueBlockChain() *BlockChain {
	// kinda feels like pointless safety, will fail in any case if the blockchain doesn't exist
	if DBexists(dbFile) == false {
		fmt.Println("No blockchain found, please create one first")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions(dbPath)
	db, err := badger.Open(opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		err = item.Value(func(val []byte) error {
			lastHash = val
			return nil
		})
		Handle(err)
		return err
	})
	Handle(err)

	chain := BlockChain{lastHash, db}
	return &chain
}

func DBexists(db string) bool {
	if _, err := os.Stat(db); os.IsNotExist(err) {
		return false
	}
	return true
}

func (chain *BlockChain) FindUnspentTransactions(address string) []Transaction {
	var unspentTxs []Transaction

	spentTXNs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		// loop through transactions
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

			// loop through outputs in transaction
		Outputs:
			for outIdx, out := range tx.Outputs {
				// txns that we have seen an input for are in this map
				if spentTXNs[txID] != nil {
					for _, spentOut := range spentTXNs[txID] {
						if spentOut == outIdx {
							// we know this txn has been spent, skip this output
							continue Outputs
						}
					}
				}
				// only gets here if we haven't seen known output
				// if our address can unlock the output (and we already know it's not spent) we add to our unspent pile
				if out.CanBeUnlocked(address) {
					unspentTxs = append(unspentTxs, *tx)
				}
			}
			if tx.IsCoinbase() == false {
				for _, in := range tx.Inputs {
					if in.CanUnlock(address) {
						// if our address can unlock the input then this transaction is spent
						inTxID := hex.EncodeToString(in.ID)
						spentTXNs[inTxID] = append(spentTXNs[inTxID], in.Out)
					}
				}
			}
			// kill the iteration as the block has no previous hash
			if len(block.PrevHash) == 0 {
				break
			}
		}
		return unspentTxs
	}
}

func (chain *BlockChain) FindUTXO(address string) []TxOutput {
	var UTXOs []TxOutput
	unspentTransactions := chain.FindUnspentTransactions(address)
	for _, tx := range unspentTransactions {
		for _, out := range tx.Outputs {
			if out.CanBeUnlocked(address) {
				UTXOs = append(UTXOs, out)
			}
		}
	}

	return UTXOs
}

func (chain *BlockChain) FindSpendableOutputs(address string, amount int) (int, map[string][]int) {
	unspentOuts := make(map[string][]int)
	unspentTxs := chain.FindUnspentTransactions(address)
	accumulated := 0

Work:
	for _, tx := range unspentTxs {
		txID := hex.EncodeToString(tx.ID)
		for outIdx, out := range tx.Outputs {
			if out.CanBeUnlocked(address) && accumulated < amount {
				accumulated += out.Value
				unspentOuts[txID] = append(unspentOuts[txID], outIdx)

				if accumulated >= amount {
					break Work
				}
			}
		}
	}
	return accumulated, unspentOuts
}

type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

func (chain *BlockChain) Iterator() *BlockChainIterator {
	iterator := BlockChainIterator{chain.LastHash, chain.Database}

	return &iterator
}

func (iterator *BlockChainIterator) Next() *Block {
	var block *Block

	err := iterator.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iterator.CurrentHash)
		Handle(err)

		err = item.Value(func(val []byte) error {
			block = Deserialize(val)
			return nil
		})
		Handle(err)
		return err
	})
	Handle(err)

	iterator.CurrentHash = block.PrevHash

	return block
}
