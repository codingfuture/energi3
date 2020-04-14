// Copyright 2019 The Energi Core Authors
// This file is part of the Energi Core library.
//
// The Energi Core library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Energi Core library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Energi Core library. If not, see <http://www.gnu.org/licenses/>.

package consensus

import (
	"sync"
	"time"

	"energi.world/core/gen3/common"
	eth_consensus "energi.world/core/gen3/consensus"
	"energi.world/core/gen3/core/types"

	energi_params "energi.world/core/gen3/energi/params"
)

const (
	oldForkPeriod = time.Duration(15) * time.Minute
)

type KnownStakeKey struct {
	coinbase common.Address
	parent   common.Hash
}
type KnownStakeValue struct {
	block  common.Hash
	expire uint64
}

func (ksv *KnownStakeValue) isActive(now uint64) bool {
	return now < ksv.expire
}

type KnownStakes = sync.Map

func (e *Energi) checkDoS(
	chain ChainReader,
	header *types.Header,
	parent *types.Header,
) error {
	old_fork_threshold := e.now() - energi_params.OldForkPeriod
	throttle := energi_params.StakeThrottle

	// POS-8: allow old fork only if current head is not fresh enough
	//
	// UPD: allow strong enough old forks to recover from heavy chain splits
	//---
	if parent.Time < old_fork_threshold {
		current := chain.CurrentHeader()

		if current.Time > old_fork_threshold {
			canonical_alt := chain.GetHeaderByNumber(header.Number.Uint64())

			if current.Difficulty.Cmp(header.Difficulty) > 0 &&
				canonical_alt != nil &&
				canonical_alt.Difficulty.Cmp(header.Difficulty) > 0 {
				return eth_consensus.ErrDoSThrottle
			}

			throttle = energi_params.OldThrottle
		}
	}

	// POS-9: stake throttling
	//---

	now := e.now()

	ksk := KnownStakeKey{
		coinbase: header.Coinbase,
		parent:   header.ParentHash,
	}
	ksv := &KnownStakeValue{
		block:  header.Hash(),
		expire: now + throttle,
	}

	if prev_ksvi, ok := e.knownStakes.LoadOrStore(ksk, ksv); ok {
		prev_ksv := prev_ksvi.(*KnownStakeValue)
		if prev_ksv.isActive(now) && prev_ksv.block != ksv.block {
			return eth_consensus.ErrDoSThrottle
		}

		e.knownStakes.Store(ksk, ksv)
	}

	//---
	if e.nextKSPurge < now {
		e.nextKSPurge = now + energi_params.StakeThrottle

		e.knownStakes.Range(func(k, v interface{}) bool {
			if !v.(*KnownStakeValue).isActive(now) {
				e.knownStakes.Delete(k)
			}

			return true
		})
	}
	//---

	return nil
}
