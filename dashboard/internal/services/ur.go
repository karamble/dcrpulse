// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"hash/crc32"
	"strings"
)

// bytewords is the BCR-2020-012 wordlist (256 four-letter words). BC-UR's
// "minimal" style encodes each byte as that word's first+last letter.
const bytewords = "ableacidalsoapexaquaarchatomauntawayaxisbackbaldbarnbeltbetabiasbluebodybragbrewbulbbuzzcalmcashcatschefcityclawcodecolacookcostcruxcurlcuspcyandarkdatadaysdelidicedietdoordowndrawdropdrumdulldutyeacheasyechoedgeepicevenexamexiteyesfactfairfernfigsfilmfishfizzflapflewfluxfoxyfreefrogfuelfundgalagamegeargemsgiftgirlglowgoodgraygrimgurugushgyrohalfhanghardhawkheathelphighhillholyhopehornhutsicedideaidleinchinkyintoirisironitemjadejazzjoinjoltjowljudojugsjumpjunkjurykeepkenokeptkeyskickkilnkingkitekiwiknoblamblavalazyleaflegsliarlimplionlistlogoloudloveluaulucklungmainmanymathmazememomenumeowmildmintmissmonknailnavyneednewsnextnoonnotenumbobeyoboeomitonyxopenovalowlspaidpartpeckplaypluspoempoolposepuffpumapurrquadquizraceramprealredorichroadrockroofrubyruinrunsrustsafesagascarsetssilkskewslotsoapsolosongstubsurfswantacotasktaxitenttiedtimetinytoiltombtoystriptunatwinuglyundouniturgeuservastveryvetovialvibeviewvisavoidvowswallwandwarmwaspwavewaxywebswhatwhenwhizwolfworkyankyawnyellyogayurtzapszerozestzinczonezoom"

// bytewordsMinimal encodes bytes as minimal-style Bytewords (2 letters per byte)
// with the BCR-2020-012 CRC-32 (big-endian) appended to the data first.
func bytewordsMinimal(data []byte) string {
	crc := crc32.ChecksumIEEE(data)
	withCRC := append(append([]byte(nil), data...),
		byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	var sb strings.Builder
	sb.Grow(len(withCRC) * 2)
	for _, b := range withCRC {
		w := int(b) * 4
		sb.WriteByte(bytewords[w])
		sb.WriteByte(bytewords[w+3])
	}
	return sb.String()
}

// EncodeUR builds a single-part BC-UR string for the given type and raw CBOR
// payload (e.g. urType "dcr-sign-request"): ur:<type>/bytewords(payload + crc32).
// The payload is the message itself - NO CBOR byte-string wrap - matching the
// foundation-ur crate the Passport uses (its single-part `UR::SinglePart` and
// Envoy's decode_single_part return the raw bytewords-decoded bytes). Returns
// canonical lowercase; the caller uppercases it for dense alphanumeric QR.
func EncodeUR(urType string, payload []byte) string {
	return "ur:" + urType + "/" + bytewordsMinimal(payload)
}
