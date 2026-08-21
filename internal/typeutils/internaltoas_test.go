// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package typeutils_test

import (
	"encoding/json"
	"errors"
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/uris"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/piprate/json-gold/ld"
	"github.com/stretchr/testify/suite"
)

type quoteContextLoader struct{}

func (quoteContextLoader) LoadDocument(uri string) (*ld.RemoteDocument, error) {
	var context map[string]any
	switch uri {
	case "https://www.w3.org/ns/activitystreams":
		context = activityStreamsContext()
	case "https://gotosocial.org/ns":
		context = map[string]any{}
	default:
		return nil, errors.New("unexpected JSON-LD context: " + uri)
	}
	return &ld.RemoteDocument{
		DocumentURL: uri,
		Document:    map[string]any{"@context": context},
	}, nil
}

func activityStreamsContext() map[string]any {
	return map[string]any{
		"@vocab": "https://www.w3.org/ns/activitystreams#",
		"id":     "@id",
		"type":   "@type",
		"content": map[string]any{
			"@id": "https://www.w3.org/ns/activitystreams#content",
		},
		"contentMap": map[string]any{
			"@id":        "https://www.w3.org/ns/activitystreams#content",
			"@container": "@language",
		},
	}
}

func mastodonQuoteContext() map[string]any {
	context := activityStreamsContext()
	for term, definition := range map[string]any{
		"quote": map[string]any{
			"@id":   "https://w3id.org/fep/044f#quote",
			"@type": "@id",
		},
		"quoteUri": map[string]any{
			"@id":   "http://fedibird.com/ns#quoteUri",
			"@type": "@id",
		},
		"_misskey_quote": map[string]any{
			"@id":   "https://misskey-hub.net/ns#_misskey_quote",
			"@type": "@id",
		},
	} {
		context[term] = definition
	}
	return context
}

type InternalToASTestSuite struct {
	TypeUtilsTestSuite
}

func (suite *InternalToASTestSuite) TestAccountToAS() {
	testAccount := &gtsmodel.Account{}
	*testAccount = *suite.testAccounts["local_account_1"] // take zork for this test

	accountable, err := suite.typeconverter.AccountToAS(suite.T().Context(), testAccount)
	suite.NoError(err)

	ser, err := ap.Serialize(accountable)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://w3id.org/security/v1",
    "https://www.w3.org/ns/activitystreams",
    {
      "discoverable": "toot:discoverable",
      "featured": {
        "@id": "toot:featured",
        "@type": "@id"
      },
      "indexable": "toot:indexable",
      "manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
      "toot": "http://joinmastodon.org/ns#"
    }
  ],
  "discoverable": true,
  "featured": "http://localhost:8080/users/the_mighty_zork/collections/featured",
  "followers": "http://localhost:8080/users/the_mighty_zork/followers",
  "following": "http://localhost:8080/users/the_mighty_zork/following",
  "hidesCcPublicFromUnauthedWeb": false,
  "hidesToPublicFromUnauthedWeb": false,
  "icon": {
    "mediaType": "image/jpeg",
    "name": "a green goblin looking nasty",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/avatar/original/01F8MH58A357CV5K7R7TJMSH6S.jpg"
  },
  "id": "http://localhost:8080/users/the_mighty_zork",
  "image": {
    "mediaType": "image/jpeg",
    "name": "A very old-school screenshot of the original team fortress mod for quake",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/header/original/01PFPMWK2FF0D9WMHEJHR07C3Q.jpg"
  },
  "inbox": "http://localhost:8080/users/the_mighty_zork/inbox",
  "indexable": true,
  "manuallyApprovesFollowers": false,
  "name": "original zork (he/they)",
  "outbox": "http://localhost:8080/users/the_mighty_zork/outbox",
  "preferredUsername": "the_mighty_zork",
  "publicKey": {
    "id": "http://localhost:8080/users/the_mighty_zork/main-key",
    "owner": "http://localhost:8080/users/the_mighty_zork",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqtQQjwFLHPez+7uF9AX7\nuvLFHm3SyNIozhhVmGhxHIs0xdgRnZKmzmZkFdrFuXddBTAglU4C2u3dw10jJO1a\nWIFQF8bGkRHZG7Pd25/XmWWBRPmOJxNLeWBqpj0G+2zTMgnAV72hALSDFY2/QDsx\nUthenKw0Srpj1LUwvRbyVQQ8fGu4v0HACFnlOX2hCQwhfAnGrb0V70Y2IJu++MP7\n6i49md0vR0Mv3WbsEJUNp1fTIUzkgWB31icvfrNmaaAxw5FkAE+KfkkylhRxi5i5\nRR1XQUINWc2Kj2Kro+CJarKG+9zasMyN7+D230gpESi8rXv1SwRu865FR3gANdDS\nMwIDAQAB\n-----END PUBLIC KEY-----\n"
  },
  "published": "2022-05-20T11:09:18Z",
  "summary": "\u003cp\u003ehey yo this is my profile!\u003c/p\u003e",
  "type": "Person",
  "url": "http://localhost:8080/@the_mighty_zork"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestAccountToASBot() {
	testAccount := &gtsmodel.Account{}
	*testAccount = *suite.testAccounts["local_account_1"] // take zork for this test

	// Update zork to be a bot.
	testAccount.ActorType = gtsmodel.AccountActorTypeService
	if err := suite.state.DB.UpdateAccount(suite.T().Context(), testAccount); err != nil {
		suite.FailNow(err.Error())
	}

	accountable, err := suite.typeconverter.AccountToAS(suite.T().Context(), testAccount)
	suite.NoError(err)

	ser, err := ap.Serialize(accountable)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://w3id.org/security/v1",
    "https://www.w3.org/ns/activitystreams",
    {
      "discoverable": "toot:discoverable",
      "featured": {
        "@id": "toot:featured",
        "@type": "@id"
      },
      "indexable": "toot:indexable",
      "manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
      "toot": "http://joinmastodon.org/ns#"
    }
  ],
  "discoverable": true,
  "featured": "http://localhost:8080/users/the_mighty_zork/collections/featured",
  "followers": "http://localhost:8080/users/the_mighty_zork/followers",
  "following": "http://localhost:8080/users/the_mighty_zork/following",
  "hidesCcPublicFromUnauthedWeb": false,
  "hidesToPublicFromUnauthedWeb": false,
  "icon": {
    "mediaType": "image/jpeg",
    "name": "a green goblin looking nasty",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/avatar/original/01F8MH58A357CV5K7R7TJMSH6S.jpg"
  },
  "id": "http://localhost:8080/users/the_mighty_zork",
  "image": {
    "mediaType": "image/jpeg",
    "name": "A very old-school screenshot of the original team fortress mod for quake",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/header/original/01PFPMWK2FF0D9WMHEJHR07C3Q.jpg"
  },
  "inbox": "http://localhost:8080/users/the_mighty_zork/inbox",
  "indexable": true,
  "manuallyApprovesFollowers": false,
  "name": "original zork (he/they)",
  "outbox": "http://localhost:8080/users/the_mighty_zork/outbox",
  "preferredUsername": "the_mighty_zork",
  "publicKey": {
    "id": "http://localhost:8080/users/the_mighty_zork/main-key",
    "owner": "http://localhost:8080/users/the_mighty_zork",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqtQQjwFLHPez+7uF9AX7\nuvLFHm3SyNIozhhVmGhxHIs0xdgRnZKmzmZkFdrFuXddBTAglU4C2u3dw10jJO1a\nWIFQF8bGkRHZG7Pd25/XmWWBRPmOJxNLeWBqpj0G+2zTMgnAV72hALSDFY2/QDsx\nUthenKw0Srpj1LUwvRbyVQQ8fGu4v0HACFnlOX2hCQwhfAnGrb0V70Y2IJu++MP7\n6i49md0vR0Mv3WbsEJUNp1fTIUzkgWB31icvfrNmaaAxw5FkAE+KfkkylhRxi5i5\nRR1XQUINWc2Kj2Kro+CJarKG+9zasMyN7+D230gpESi8rXv1SwRu865FR3gANdDS\nMwIDAQAB\n-----END PUBLIC KEY-----\n"
  },
  "published": "2022-05-20T11:09:18Z",
  "summary": "\u003cp\u003ehey yo this is my profile!\u003c/p\u003e",
  "type": "Service",
  "url": "http://localhost:8080/@the_mighty_zork"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestAccountToASWithFields() {
	testAccount := &gtsmodel.Account{}
	*testAccount = *suite.testAccounts["local_account_2"]

	accountable, err := suite.typeconverter.AccountToAS(suite.T().Context(), testAccount)
	suite.NoError(err)

	ser, err := ap.Serialize(accountable)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://w3id.org/security/v1",
    "https://www.w3.org/ns/activitystreams",
    {
      "PropertyValue": "schema:PropertyValue",
      "discoverable": "toot:discoverable",
      "featured": {
        "@id": "toot:featured",
        "@type": "@id"
      },
      "indexable": "toot:indexable",
      "manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
      "schema": "http://schema.org#",
      "toot": "http://joinmastodon.org/ns#",
      "value": "schema:value"
    }
  ],
  "attachment": [
    {
      "name": "should you follow me?",
      "type": "PropertyValue",
      "value": "maybe!"
    },
    {
      "name": "age",
      "type": "PropertyValue",
      "value": "120"
    }
  ],
  "discoverable": false,
  "featured": "http://localhost:8080/users/1happyturtle/collections/featured",
  "followers": "http://localhost:8080/users/1happyturtle/followers",
  "following": "http://localhost:8080/users/1happyturtle/following",
  "hidesCcPublicFromUnauthedWeb": true,
  "hidesToPublicFromUnauthedWeb": false,
  "id": "http://localhost:8080/users/1happyturtle",
  "inbox": "http://localhost:8080/users/1happyturtle/inbox",
  "indexable": false,
  "manuallyApprovesFollowers": true,
  "name": "happy little turtle :3",
  "outbox": "http://localhost:8080/users/1happyturtle/outbox",
  "preferredUsername": "1happyturtle",
  "publicKey": {
    "id": "http://localhost:8080/users/1happyturtle#main-key",
    "owner": "http://localhost:8080/users/1happyturtle",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAwXTcOAvM1Jiw5Ffpk0qn\nr0cwbNvFe/5zQ+Tp7tumK/ZnT37o7X0FUEXrxNi+dkhmeJ0gsaiN+JQGNUewvpSk\nPIAXKvi908aSfCGjs7bGlJCJCuDuL5d6m7hZnP9rt9fJc70GElPpG0jc9fXwlz7T\nlsPb2ecatmG05Y4jPwdC+oN4MNCv9yQzEvCVMzl76EJaM602kIHC1CISn0rDFmYd\n9rSN7XPlNJw1F6PbpJ/BWQ+pXHKw3OEwNTETAUNYiVGnZU+B7a7bZC9f6/aPbJuV\nt8Qmg+UnDvW1Y8gmfHnxaWG2f5TDBvCHmcYtucIZPLQD4trAozC4ryqlmCWQNKbt\n0wIDAQAB\n-----END PUBLIC KEY-----\n"
  },
  "published": "2022-06-04T13:12:00Z",
  "summary": "\u003cp\u003ei post about things that concern me\u003c/p\u003e",
  "type": "Person",
  "url": "http://localhost:8080/@1happyturtle"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestAccountToASAliasedAndMoved() {
	testAccount := &gtsmodel.Account{}
	*testAccount = *suite.testAccounts["local_account_1"] // take zork for this test

	ctx := suite.T().Context()

	// Suppose zork has moved account to turtle.
	testAccount.AlsoKnownAsURIs = []string{"http://localhost:8080/users/1happyturtle"}
	testAccount.MovedToURI = "http://localhost:8080/users/1happyturtle"
	if err := suite.state.DB.UpdateAccount(ctx,
		testAccount,
		"also_known_as_uris",
		"moved_to_uri",
	); err != nil {
		suite.FailNow(err.Error())
	}

	accountable, err := suite.typeconverter.AccountToAS(suite.T().Context(), testAccount)
	suite.NoError(err)

	ser, err := ap.Serialize(accountable)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://w3id.org/security/v1",
    "https://www.w3.org/ns/activitystreams",
    {
      "discoverable": "toot:discoverable",
      "featured": {
        "@id": "toot:featured",
        "@type": "@id"
      },
      "indexable": "toot:indexable",
      "manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
      "movedTo": {
        "@id": "as:movedTo",
        "@type": "@id"
      },
      "toot": "http://joinmastodon.org/ns#"
    }
  ],
  "alsoKnownAs": [
    "http://localhost:8080/users/1happyturtle"
  ],
  "discoverable": true,
  "featured": "http://localhost:8080/users/the_mighty_zork/collections/featured",
  "followers": "http://localhost:8080/users/the_mighty_zork/followers",
  "following": "http://localhost:8080/users/the_mighty_zork/following",
  "hidesCcPublicFromUnauthedWeb": false,
  "hidesToPublicFromUnauthedWeb": false,
  "icon": {
    "mediaType": "image/jpeg",
    "name": "a green goblin looking nasty",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/avatar/original/01F8MH58A357CV5K7R7TJMSH6S.jpg"
  },
  "id": "http://localhost:8080/users/the_mighty_zork",
  "image": {
    "mediaType": "image/jpeg",
    "name": "A very old-school screenshot of the original team fortress mod for quake",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/header/original/01PFPMWK2FF0D9WMHEJHR07C3Q.jpg"
  },
  "inbox": "http://localhost:8080/users/the_mighty_zork/inbox",
  "indexable": true,
  "manuallyApprovesFollowers": false,
  "movedTo": "http://localhost:8080/users/1happyturtle",
  "name": "original zork (he/they)",
  "outbox": "http://localhost:8080/users/the_mighty_zork/outbox",
  "preferredUsername": "the_mighty_zork",
  "publicKey": {
    "id": "http://localhost:8080/users/the_mighty_zork/main-key",
    "owner": "http://localhost:8080/users/the_mighty_zork",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqtQQjwFLHPez+7uF9AX7\nuvLFHm3SyNIozhhVmGhxHIs0xdgRnZKmzmZkFdrFuXddBTAglU4C2u3dw10jJO1a\nWIFQF8bGkRHZG7Pd25/XmWWBRPmOJxNLeWBqpj0G+2zTMgnAV72hALSDFY2/QDsx\nUthenKw0Srpj1LUwvRbyVQQ8fGu4v0HACFnlOX2hCQwhfAnGrb0V70Y2IJu++MP7\n6i49md0vR0Mv3WbsEJUNp1fTIUzkgWB31icvfrNmaaAxw5FkAE+KfkkylhRxi5i5\nRR1XQUINWc2Kj2Kro+CJarKG+9zasMyN7+D230gpESi8rXv1SwRu865FR3gANdDS\nMwIDAQAB\n-----END PUBLIC KEY-----\n"
  },
  "published": "2022-05-20T11:09:18Z",
  "summary": "\u003cp\u003ehey yo this is my profile!\u003c/p\u003e",
  "type": "Person",
  "url": "http://localhost:8080/@the_mighty_zork"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestAccountToASWithOneField() {
	testAccount := &gtsmodel.Account{}
	*testAccount = *suite.testAccounts["local_account_2"]
	testAccount.Fields = testAccount.Fields[0:1] // Take only one field.

	accountable, err := suite.typeconverter.AccountToAS(suite.T().Context(), testAccount)
	suite.NoError(err)

	ser, err := ap.Serialize(accountable)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	// Despite only one field being set, attachments should still be a slice/array.
	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://w3id.org/security/v1",
    "https://www.w3.org/ns/activitystreams",
    {
      "PropertyValue": "schema:PropertyValue",
      "discoverable": "toot:discoverable",
      "featured": {
        "@id": "toot:featured",
        "@type": "@id"
      },
      "indexable": "toot:indexable",
      "manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
      "schema": "http://schema.org#",
      "toot": "http://joinmastodon.org/ns#",
      "value": "schema:value"
    }
  ],
  "attachment": [
    {
      "name": "should you follow me?",
      "type": "PropertyValue",
      "value": "maybe!"
    }
  ],
  "discoverable": false,
  "featured": "http://localhost:8080/users/1happyturtle/collections/featured",
  "followers": "http://localhost:8080/users/1happyturtle/followers",
  "following": "http://localhost:8080/users/1happyturtle/following",
  "hidesCcPublicFromUnauthedWeb": true,
  "hidesToPublicFromUnauthedWeb": false,
  "id": "http://localhost:8080/users/1happyturtle",
  "inbox": "http://localhost:8080/users/1happyturtle/inbox",
  "indexable": false,
  "manuallyApprovesFollowers": true,
  "name": "happy little turtle :3",
  "outbox": "http://localhost:8080/users/1happyturtle/outbox",
  "preferredUsername": "1happyturtle",
  "publicKey": {
    "id": "http://localhost:8080/users/1happyturtle#main-key",
    "owner": "http://localhost:8080/users/1happyturtle",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAwXTcOAvM1Jiw5Ffpk0qn\nr0cwbNvFe/5zQ+Tp7tumK/ZnT37o7X0FUEXrxNi+dkhmeJ0gsaiN+JQGNUewvpSk\nPIAXKvi908aSfCGjs7bGlJCJCuDuL5d6m7hZnP9rt9fJc70GElPpG0jc9fXwlz7T\nlsPb2ecatmG05Y4jPwdC+oN4MNCv9yQzEvCVMzl76EJaM602kIHC1CISn0rDFmYd\n9rSN7XPlNJw1F6PbpJ/BWQ+pXHKw3OEwNTETAUNYiVGnZU+B7a7bZC9f6/aPbJuV\nt8Qmg+UnDvW1Y8gmfHnxaWG2f5TDBvCHmcYtucIZPLQD4trAozC4ryqlmCWQNKbt\n0wIDAQAB\n-----END PUBLIC KEY-----\n"
  },
  "published": "2022-06-04T13:12:00Z",
  "summary": "\u003cp\u003ei post about things that concern me\u003c/p\u003e",
  "type": "Person",
  "url": "http://localhost:8080/@1happyturtle"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestAccountToASWithEmoji() {
	testAccount := &gtsmodel.Account{}
	*testAccount = *suite.testAccounts["local_account_1"] // take zork for this test
	testAccount.Emojis = []*gtsmodel.Emoji{suite.testEmojis["rainbow"]}

	accountable, err := suite.typeconverter.AccountToAS(suite.T().Context(), testAccount)
	suite.NoError(err)

	ser, err := ap.Serialize(accountable)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://w3id.org/security/v1",
    "https://www.w3.org/ns/activitystreams",
    {
      "Emoji": "toot:Emoji",
      "discoverable": "toot:discoverable",
      "featured": {
        "@id": "toot:featured",
        "@type": "@id"
      },
      "indexable": "toot:indexable",
      "manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
      "toot": "http://joinmastodon.org/ns#"
    }
  ],
  "discoverable": true,
  "featured": "http://localhost:8080/users/the_mighty_zork/collections/featured",
  "followers": "http://localhost:8080/users/the_mighty_zork/followers",
  "following": "http://localhost:8080/users/the_mighty_zork/following",
  "hidesCcPublicFromUnauthedWeb": false,
  "hidesToPublicFromUnauthedWeb": false,
  "icon": {
    "mediaType": "image/jpeg",
    "name": "a green goblin looking nasty",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/avatar/original/01F8MH58A357CV5K7R7TJMSH6S.jpg"
  },
  "id": "http://localhost:8080/users/the_mighty_zork",
  "image": {
    "mediaType": "image/jpeg",
    "name": "A very old-school screenshot of the original team fortress mod for quake",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/header/original/01PFPMWK2FF0D9WMHEJHR07C3Q.jpg"
  },
  "inbox": "http://localhost:8080/users/the_mighty_zork/inbox",
  "indexable": true,
  "manuallyApprovesFollowers": false,
  "name": "original zork (he/they)",
  "outbox": "http://localhost:8080/users/the_mighty_zork/outbox",
  "preferredUsername": "the_mighty_zork",
  "publicKey": {
    "id": "http://localhost:8080/users/the_mighty_zork/main-key",
    "owner": "http://localhost:8080/users/the_mighty_zork",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqtQQjwFLHPez+7uF9AX7\nuvLFHm3SyNIozhhVmGhxHIs0xdgRnZKmzmZkFdrFuXddBTAglU4C2u3dw10jJO1a\nWIFQF8bGkRHZG7Pd25/XmWWBRPmOJxNLeWBqpj0G+2zTMgnAV72hALSDFY2/QDsx\nUthenKw0Srpj1LUwvRbyVQQ8fGu4v0HACFnlOX2hCQwhfAnGrb0V70Y2IJu++MP7\n6i49md0vR0Mv3WbsEJUNp1fTIUzkgWB31icvfrNmaaAxw5FkAE+KfkkylhRxi5i5\nRR1XQUINWc2Kj2Kro+CJarKG+9zasMyN7+D230gpESi8rXv1SwRu865FR3gANdDS\nMwIDAQAB\n-----END PUBLIC KEY-----\n"
  },
  "published": "2022-05-20T11:09:18Z",
  "summary": "\u003cp\u003ehey yo this is my profile!\u003c/p\u003e",
  "tag": {
    "icon": {
      "mediaType": "image/png",
      "type": "Image",
      "url": "http://localhost:8080/fileserver/01AY6P665V14JJR0AFVRT7311Y/emoji/original/01F8MH9H8E4VG3KDYJR9EGPXCQ.png"
    },
    "id": "http://localhost:8080/emoji/01F8MH9H8E4VG3KDYJR9EGPXCQ",
    "name": ":rainbow:",
    "type": "Emoji",
    "updated": "2021-09-20T12:40:37+02:00"
  },
  "type": "Person",
  "url": "http://localhost:8080/@the_mighty_zork"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestAccountToASWithSharedInbox() {
	testAccount := &gtsmodel.Account{}
	*testAccount = *suite.testAccounts["local_account_1"] // take zork for this test
	sharedInbox := "http://localhost:8080/sharedInbox"
	testAccount.SharedInboxURI = &sharedInbox

	accountable, err := suite.typeconverter.AccountToAS(suite.T().Context(), testAccount)
	suite.NoError(err)

	ser, err := ap.Serialize(accountable)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://w3id.org/security/v1",
    "https://www.w3.org/ns/activitystreams",
    {
      "discoverable": "toot:discoverable",
      "featured": {
        "@id": "toot:featured",
        "@type": "@id"
      },
      "indexable": "toot:indexable",
      "manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
      "toot": "http://joinmastodon.org/ns#"
    }
  ],
  "discoverable": true,
  "endpoints": {
    "sharedInbox": "http://localhost:8080/sharedInbox"
  },
  "featured": "http://localhost:8080/users/the_mighty_zork/collections/featured",
  "followers": "http://localhost:8080/users/the_mighty_zork/followers",
  "following": "http://localhost:8080/users/the_mighty_zork/following",
  "hidesCcPublicFromUnauthedWeb": false,
  "hidesToPublicFromUnauthedWeb": false,
  "icon": {
    "mediaType": "image/jpeg",
    "name": "a green goblin looking nasty",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/avatar/original/01F8MH58A357CV5K7R7TJMSH6S.jpg"
  },
  "id": "http://localhost:8080/users/the_mighty_zork",
  "image": {
    "mediaType": "image/jpeg",
    "name": "A very old-school screenshot of the original team fortress mod for quake",
    "type": "Image",
    "url": "http://localhost:8080/fileserver/01F8MH1H7YV1Z7D2C8K2730QBF/header/original/01PFPMWK2FF0D9WMHEJHR07C3Q.jpg"
  },
  "inbox": "http://localhost:8080/users/the_mighty_zork/inbox",
  "indexable": true,
  "manuallyApprovesFollowers": false,
  "name": "original zork (he/they)",
  "outbox": "http://localhost:8080/users/the_mighty_zork/outbox",
  "preferredUsername": "the_mighty_zork",
  "publicKey": {
    "id": "http://localhost:8080/users/the_mighty_zork/main-key",
    "owner": "http://localhost:8080/users/the_mighty_zork",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqtQQjwFLHPez+7uF9AX7\nuvLFHm3SyNIozhhVmGhxHIs0xdgRnZKmzmZkFdrFuXddBTAglU4C2u3dw10jJO1a\nWIFQF8bGkRHZG7Pd25/XmWWBRPmOJxNLeWBqpj0G+2zTMgnAV72hALSDFY2/QDsx\nUthenKw0Srpj1LUwvRbyVQQ8fGu4v0HACFnlOX2hCQwhfAnGrb0V70Y2IJu++MP7\n6i49md0vR0Mv3WbsEJUNp1fTIUzkgWB31icvfrNmaaAxw5FkAE+KfkkylhRxi5i5\nRR1XQUINWc2Kj2Kro+CJarKG+9zasMyN7+D230gpESi8rXv1SwRu865FR3gANdDS\nMwIDAQAB\n-----END PUBLIC KEY-----\n"
  },
  "published": "2022-05-20T11:09:18Z",
  "summary": "\u003cp\u003ehey yo this is my profile!\u003c/p\u003e",
  "type": "Person",
  "url": "http://localhost:8080/@the_mighty_zork"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestStatusToAS() {
	testStatus := suite.testStatuses["local_account_1_status_1"]
	ctx := suite.T().Context()

	asStatus, err := suite.typeconverter.StatusToAS(ctx, testStatus)
	suite.NoError(err)

	ser, err := ap.Serialize(asStatus)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://www.w3.org/ns/activitystreams",
    {
      "sensitive": "as:sensitive"
    }
  ],
  "attributedTo": "http://localhost:8080/users/the_mighty_zork",
  "cc": "http://localhost:8080/users/the_mighty_zork/followers",
  "content": "\u003cp\u003ehello everyone!\u003c/p\u003e",
  "contentMap": {
    "en": "\u003cp\u003ehello everyone!\u003c/p\u003e"
  },
  "id": "http://localhost:8080/users/the_mighty_zork/statuses/01F8MHAMCHF6Y650WCRSCP4WMY",
  "interactionPolicy": {
    "canAnnounce": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canLike": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canQuote": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canReply": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    }
  },
  "published": "2021-10-20T12:40:37+02:00",
  "replies": {
    "first": {
      "id": "http://localhost:8080/users/the_mighty_zork/statuses/01F8MHAMCHF6Y650WCRSCP4WMY/replies?page=true",
      "next": "http://localhost:8080/users/the_mighty_zork/statuses/01F8MHAMCHF6Y650WCRSCP4WMY/replies?page=true\u0026only_other_accounts=false",
      "partOf": "http://localhost:8080/users/the_mighty_zork/statuses/01F8MHAMCHF6Y650WCRSCP4WMY/replies",
      "type": "CollectionPage"
    },
    "id": "http://localhost:8080/users/the_mighty_zork/statuses/01F8MHAMCHF6Y650WCRSCP4WMY/replies",
    "type": "Collection"
  },
  "sensitive": true,
  "summary": "introduction post",
  "to": "https://www.w3.org/ns/activitystreams#Public",
  "type": "Note",
  "url": "http://localhost:8080/@the_mighty_zork/statuses/01F8MHAMCHF6Y650WCRSCP4WMY"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestQuoteStatusToASPreservesContentShape() {
	testStatus := new(gtsmodel.Status)
	*testStatus = *suite.testStatuses["local_account_1_status_1"]
	testStatus.QuoteURI = "https://remote.example/users/alice/statuses/quoted"

	asStatus, err := suite.typeconverter.StatusToAS(suite.T().Context(), testStatus)
	suite.NoError(err)

	serialized, err := ap.Serialize(asStatus)
	suite.NoError(err)
	suite.Equal("<p>hello everyone!</p>", serialized["content"])
	suite.Equal(
		map[string]string{"en": "<p>hello everyone!</p>"},
		serialized["contentMap"],
	)
	suite.Equal(testStatus.QuoteURI, serialized["quote"])
	suite.Equal(testStatus.QuoteURI, serialized["quoteUri"])
	suite.Equal(testStatus.QuoteURI, serialized["_misskey_quote"])

	context, ok := serialized["@context"].([]any)
	suite.True(ok)
	inline, ok := context[len(context)-1].(map[string]any)
	suite.True(ok)
	suite.Equal(
		map[string]string{
			"@id":   "https://w3id.org/fep/044f#quote",
			"@type": "@id",
		},
		inline["quote"],
	)
	suite.Equal(
		map[string]string{
			"@id":   "http://fedibird.com/ns#quoteUri",
			"@type": "@id",
		},
		inline["quoteUri"],
	)
	suite.Equal(
		map[string]string{
			"@id":   "https://misskey-hub.net/ns#_misskey_quote",
			"@type": "@id",
		},
		inline["_misskey_quote"],
	)

	// Run a real JSON-LD expand/compact normalization using the relevant
	// ActivityStreams and Mastodon quote term definitions. Remote contexts are
	// supplied locally so the regression test is deterministic and offline.
	options := ld.NewJsonLdOptions("")
	options.DocumentLoader = quoteContextLoader{}
	processor := ld.NewJsonLdProcessor()
	wire, err := json.Marshal(serialized)
	suite.NoError(err)
	var jsonDocument any
	suite.NoError(json.Unmarshal(wire, &jsonDocument))
	expanded, err := processor.Expand(jsonDocument, options)
	suite.NoError(err)
	compacted, err := processor.Compact(expanded, map[string]any{"@context": mastodonQuoteContext()}, options)
	suite.NoError(err)
	suite.IsType("", compacted["content"])
	suite.Equal("<p>hello everyone!</p>", compacted["content"])
	suite.IsType(map[string]any{}, compacted["contentMap"])
	suite.Equal(map[string]any{"en": "<p>hello everyone!</p>"}, compacted["contentMap"])
	suite.Equal(testStatus.QuoteURI, compacted["quote"])
	suite.Equal(testStatus.QuoteURI, compacted["quoteUri"])
	suite.Equal(testStatus.QuoteURI, compacted["_misskey_quote"])
}

func (suite *InternalToASTestSuite) TestStatusWithTagsToASWithIDs() {
	// use the status with just IDs of attachments and emojis pinned on it
	testStatus := suite.testStatuses["admin_account_status_1"]
	ctx := suite.T().Context()

	asStatus, err := suite.typeconverter.StatusToAS(ctx, testStatus)
	suite.NoError(err)

	ser, err := ap.Serialize(asStatus)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://www.w3.org/ns/activitystreams",
    {
      "Emoji": "toot:Emoji",
      "Hashtag": "as:Hashtag",
      "blurhash": "toot:blurhash",
      "focalPoint": {
        "@container": "@list",
        "@id": "toot:focalPoint"
      },
      "toot": "http://joinmastodon.org/ns#"
    }
  ],
  "attachment": [
    {
      "blurhash": "LIIE|gRj00WB-;j[t7j[4nWBj[Rj",
      "focalPoint": [
        -0.5,
        0.5
      ],
      "mediaType": "image/jpeg",
      "name": "Black and white image of some 50's style text saying: Welcome On Board",
      "type": "Image",
      "url": "http://localhost:8080/fileserver/01F8MH17FWEB39HZJ76B6VXSKF/attachment/original/01F8MH6NEM8D7527KZAECTCR76.jpg"
    }
  ],
  "attributedTo": "http://localhost:8080/users/admin",
  "cc": "http://localhost:8080/users/admin/followers",
  "content": "\u003cp\u003ehello world! \u003ca href=\"http://localhost:8080/tags/welcome\" class=\"mention hashtag\" rel=\"tag nofollow noreferrer noopener\" target=\"_blank\"\u003e#\u003cspan\u003ewelcome\u003c/span\u003e\u003c/a\u003e ! first post on the instance :rainbow: !\u003c/p\u003e",
  "contentMap": {
    "en": "\u003cp\u003ehello world! \u003ca href=\"http://localhost:8080/tags/welcome\" class=\"mention hashtag\" rel=\"tag nofollow noreferrer noopener\" target=\"_blank\"\u003e#\u003cspan\u003ewelcome\u003c/span\u003e\u003c/a\u003e ! first post on the instance :rainbow: !\u003c/p\u003e"
  },
  "id": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R",
  "interactionPolicy": {
    "canAnnounce": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canLike": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canQuote": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canReply": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    }
  },
  "published": "2021-10-20T11:36:45Z",
  "replies": {
    "first": {
      "id": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R/replies?page=true",
      "next": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R/replies?page=true\u0026only_other_accounts=false",
      "partOf": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R/replies",
      "type": "CollectionPage"
    },
    "id": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R/replies",
    "type": "Collection"
  },
  "tag": [
    {
      "icon": {
        "mediaType": "image/png",
        "type": "Image",
        "url": "http://localhost:8080/fileserver/01AY6P665V14JJR0AFVRT7311Y/emoji/original/01F8MH9H8E4VG3KDYJR9EGPXCQ.png"
      },
      "id": "http://localhost:8080/emoji/01F8MH9H8E4VG3KDYJR9EGPXCQ",
      "name": ":rainbow:",
      "type": "Emoji",
      "updated": "2021-09-20T10:40:37Z"
    },
    {
      "href": "http://localhost:8080/tags/welcome",
      "name": "#welcome",
      "type": "Hashtag"
    }
  ],
  "to": "https://www.w3.org/ns/activitystreams#Public",
  "type": "Note",
  "url": "http://localhost:8080/@admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestStatusWithTagsToASFromDB() {
	ctx := suite.T().Context()
	// get the entire status with all tags
	testStatus, err := suite.db.GetStatusByID(ctx, suite.testStatuses["admin_account_status_1"].ID)
	suite.NoError(err)

	asStatus, err := suite.typeconverter.StatusToAS(ctx, testStatus)
	suite.NoError(err)

	ser, err := ap.Serialize(asStatus)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://www.w3.org/ns/activitystreams",
    {
      "Emoji": "toot:Emoji",
      "Hashtag": "as:Hashtag",
      "blurhash": "toot:blurhash",
      "focalPoint": {
        "@container": "@list",
        "@id": "toot:focalPoint"
      },
      "toot": "http://joinmastodon.org/ns#"
    }
  ],
  "attachment": [
    {
      "blurhash": "LIIE|gRj00WB-;j[t7j[4nWBj[Rj",
      "focalPoint": [
        -0.5,
        0.5
      ],
      "mediaType": "image/jpeg",
      "name": "Black and white image of some 50's style text saying: Welcome On Board",
      "type": "Image",
      "url": "http://localhost:8080/fileserver/01F8MH17FWEB39HZJ76B6VXSKF/attachment/original/01F8MH6NEM8D7527KZAECTCR76.jpg"
    }
  ],
  "attributedTo": "http://localhost:8080/users/admin",
  "cc": "http://localhost:8080/users/admin/followers",
  "content": "\u003cp\u003ehello world! \u003ca href=\"http://localhost:8080/tags/welcome\" class=\"mention hashtag\" rel=\"tag nofollow noreferrer noopener\" target=\"_blank\"\u003e#\u003cspan\u003ewelcome\u003c/span\u003e\u003c/a\u003e ! first post on the instance :rainbow: !\u003c/p\u003e",
  "contentMap": {
    "en": "\u003cp\u003ehello world! \u003ca href=\"http://localhost:8080/tags/welcome\" class=\"mention hashtag\" rel=\"tag nofollow noreferrer noopener\" target=\"_blank\"\u003e#\u003cspan\u003ewelcome\u003c/span\u003e\u003c/a\u003e ! first post on the instance :rainbow: !\u003c/p\u003e"
  },
  "id": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R",
  "interactionPolicy": {
    "canAnnounce": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canLike": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canQuote": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canReply": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    }
  },
  "published": "2021-10-20T11:36:45Z",
  "replies": {
    "first": {
      "id": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R/replies?page=true",
      "next": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R/replies?page=true\u0026only_other_accounts=false",
      "partOf": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R/replies",
      "type": "CollectionPage"
    },
    "id": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R/replies",
    "type": "Collection"
  },
  "tag": [
    {
      "icon": {
        "mediaType": "image/png",
        "type": "Image",
        "url": "http://localhost:8080/fileserver/01AY6P665V14JJR0AFVRT7311Y/emoji/original/01F8MH9H8E4VG3KDYJR9EGPXCQ.png"
      },
      "id": "http://localhost:8080/emoji/01F8MH9H8E4VG3KDYJR9EGPXCQ",
      "name": ":rainbow:",
      "type": "Emoji",
      "updated": "2021-09-20T10:40:37Z"
    },
    {
      "href": "http://localhost:8080/tags/welcome",
      "name": "#welcome",
      "type": "Hashtag"
    }
  ],
  "to": "https://www.w3.org/ns/activitystreams#Public",
  "type": "Note",
  "url": "http://localhost:8080/@admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestStatusToASWithMentions() {
	testStatusID := suite.testStatuses["admin_account_status_3"].ID
	ctx := suite.T().Context()

	testStatus, err := suite.db.GetStatusByID(ctx, testStatusID)
	suite.NoError(err)

	// Update interaction policy to allow only author + mentioned to reply.
	testStatus.InteractionPolicy = gtsmodel.DefaultInteractionPolicyPublic()
	testStatus.InteractionPolicy.CanReply.AutomaticApproval = gtsmodel.PolicyValues{gtsmodel.PolicyValueAuthor, gtsmodel.PolicyValueMentioned}
	testStatus.InteractionPolicy.CanReply.ManualApproval = gtsmodel.PolicyValues{gtsmodel.PolicyValuePublic}

	asStatus, err := suite.typeconverter.StatusToAS(ctx, testStatus)
	suite.NoError(err)

	ser, err := ap.Serialize(asStatus)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://www.w3.org/ns/activitystreams"
  ],
  "attributedTo": "http://localhost:8080/users/admin",
  "cc": [
    "http://localhost:8080/users/admin/followers",
    "http://localhost:8080/users/the_mighty_zork"
  ],
  "content": "\u003cp\u003ehi \u003cspan class=\"h-card\"\u003e\u003ca href=\"http://localhost:8080/@1happyturtle\" class=\"u-url mention\" rel=\"nofollow noreferrer noopener\" target=\"_blank\"\u003e@\u003cspan\u003ethe_mighty_zork\u003c/span\u003e\u003c/a\u003e\u003c/span\u003e welcome to the instance!\u003c/p\u003e",
  "contentMap": {
    "en": "\u003cp\u003ehi \u003cspan class=\"h-card\"\u003e\u003ca href=\"http://localhost:8080/@1happyturtle\" class=\"u-url mention\" rel=\"nofollow noreferrer noopener\" target=\"_blank\"\u003e@\u003cspan\u003ethe_mighty_zork\u003c/span\u003e\u003c/a\u003e\u003c/span\u003e welcome to the instance!\u003c/p\u003e"
  },
  "id": "http://localhost:8080/users/admin/statuses/01FF25D5Q0DH7CHD57CTRS6WK0",
  "inReplyTo": "http://localhost:8080/users/the_mighty_zork/statuses/01F8MHAMCHF6Y650WCRSCP4WMY",
  "interactionPolicy": {
    "canAnnounce": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canLike": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canQuote": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canReply": {
      "automaticApproval": [
        "http://localhost:8080/users/admin",
        "http://localhost:8080/users/the_mighty_zork"
      ],
      "manualApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    }
  },
  "published": "2021-11-20T13:32:16Z",
  "replies": {
    "first": {
      "id": "http://localhost:8080/users/admin/statuses/01FF25D5Q0DH7CHD57CTRS6WK0/replies?page=true",
      "next": "http://localhost:8080/users/admin/statuses/01FF25D5Q0DH7CHD57CTRS6WK0/replies?page=true\u0026only_other_accounts=false",
      "partOf": "http://localhost:8080/users/admin/statuses/01FF25D5Q0DH7CHD57CTRS6WK0/replies",
      "type": "CollectionPage"
    },
    "id": "http://localhost:8080/users/admin/statuses/01FF25D5Q0DH7CHD57CTRS6WK0/replies",
    "type": "Collection"
  },
  "tag": {
    "href": "http://localhost:8080/users/the_mighty_zork",
    "name": "@the_mighty_zork@localhost:8080",
    "type": "Mention"
  },
  "to": "https://www.w3.org/ns/activitystreams#Public",
  "type": "Note",
  "url": "http://localhost:8080/@admin/statuses/01FF25D5Q0DH7CHD57CTRS6WK0"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestStatusToASPoliteApproved() {
	ctx := suite.T().Context()

	// Take a status from admin that replies to turtle.
	testStatus := new(gtsmodel.Status)
	*testStatus = *suite.testStatuses["admin_account_status_5"]

	// Take corresponding interaction request.
	intReq := new(gtsmodel.InteractionRequest)
	*intReq = *suite.testInteractionRequests["admin_account_reply_turtle"]

	// Mark the status as approved by updating the
	// status + corresponding interaction request.
	username := suite.testAccounts["local_account_2"].Username
	intReq.ResponseURI = uris.GenerateURIForAccept(
		username,
		intReq.ID,
	)
	intReq.AuthorizationURI = uris.GenerateURIForAuthorization(
		username,
		intReq.ID,
	)
	intReq.AcceptedAt = testrig.TimeMustParse("2024-11-01T11:00:00+02:00")

	// Mark it as polite too.
	intReq.Polite = util.Ptr(true)

	if err := suite.state.DB.UpdateInteractionRequest(
		ctx,
		intReq,
		"response_uri",
		"authorization_uri",
		"accepted_at",
		"polite",
	); err != nil {
		suite.FailNow(err.Error())
	}

	testStatus.ApprovedByURI = intReq.AuthorizationURI
	if err := suite.state.DB.UpdateStatus(
		ctx,
		testStatus,
		"approved_by_uri",
	); err != nil {
		suite.FailNow(err.Error())
	}

	asStatus, err := suite.typeconverter.StatusToAS(ctx, testStatus)
	suite.NoError(err)

	ser, err := ap.Serialize(asStatus)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://www.w3.org/ns/activitystreams"
  ],
  "attributedTo": "http://localhost:8080/users/admin",
  "cc": [
    "http://localhost:8080/users/admin/followers",
    "http://localhost:8080/users/1happyturtle"
  ],
  "content": "\u003cp\u003eHi \u003cspan class=\"h-card\"\u003e\u003ca href=\"http://localhost:8080/@1happyturtle\" class=\"u-url mention\" rel=\"nofollow noreferrer noopener\" target=\"_blank\"\u003e@\u003cspan\u003e1happyturtle\u003c/span\u003e\u003c/a\u003e\u003c/span\u003e, can I reply?\u003c/p\u003e",
  "id": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ",
  "inReplyTo": "http://localhost:8080/users/1happyturtle/statuses/01F8MHC8VWDRBQR0N1BATDDEM5",
  "interactionPolicy": {
    "canAnnounce": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canLike": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canQuote": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canReply": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    }
  },
  "published": "2024-02-20T12:41:37+02:00",
  "replies": {
    "first": {
      "id": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ/replies?page=true",
      "next": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ/replies?page=true\u0026only_other_accounts=false",
      "partOf": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ/replies",
      "type": "CollectionPage"
    },
    "id": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ/replies",
    "type": "Collection"
  },
  "replyAuthorization": "http://localhost:8080/users/1happyturtle/authorizations/01J5QVXCCEATJYSXM9H6MZT4JR",
  "tag": {
    "href": "http://localhost:8080/users/1happyturtle",
    "name": "@1happyturtle@localhost:8080",
    "type": "Mention"
  },
  "to": "https://www.w3.org/ns/activitystreams#Public",
  "type": "Note",
  "url": "http://localhost:8080/@admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestStatusToASPImpoliteApproved() {
	ctx := suite.T().Context()

	// Take a status from admin that replies to turtle.
	testStatus := new(gtsmodel.Status)
	*testStatus = *suite.testStatuses["admin_account_status_5"]

	// Take corresponding interaction request.
	intReq := new(gtsmodel.InteractionRequest)
	*intReq = *suite.testInteractionRequests["admin_account_reply_turtle"]

	// Mark the status as approved by updating the
	// status + corresponding interaction request.
	username := suite.testAccounts["local_account_2"].Username
	intReq.ResponseURI = uris.GenerateURIForAccept(
		username,
		intReq.ID,
	)
	intReq.AuthorizationURI = uris.GenerateURIForAuthorization(
		username,
		intReq.ID,
	)
	intReq.AcceptedAt = testrig.TimeMustParse("2024-11-01T11:00:00+02:00")

	if err := suite.state.DB.UpdateInteractionRequest(
		ctx,
		intReq,
		"response_uri",
		"authorization_uri",
		"accepted_at",
	); err != nil {
		suite.FailNow(err.Error())
	}

	testStatus.ApprovedByURI = intReq.AuthorizationURI
	if err := suite.state.DB.UpdateStatus(
		ctx,
		testStatus,
		"approved_by_uri",
	); err != nil {
		suite.FailNow(err.Error())
	}

	asStatus, err := suite.typeconverter.StatusToAS(ctx, testStatus)
	suite.NoError(err)

	ser, err := ap.Serialize(asStatus)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://www.w3.org/ns/activitystreams"
  ],
  "attributedTo": "http://localhost:8080/users/admin",
  "cc": [
    "http://localhost:8080/users/admin/followers",
    "http://localhost:8080/users/1happyturtle"
  ],
  "content": "\u003cp\u003eHi \u003cspan class=\"h-card\"\u003e\u003ca href=\"http://localhost:8080/@1happyturtle\" class=\"u-url mention\" rel=\"nofollow noreferrer noopener\" target=\"_blank\"\u003e@\u003cspan\u003e1happyturtle\u003c/span\u003e\u003c/a\u003e\u003c/span\u003e, can I reply?\u003c/p\u003e",
  "id": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ",
  "inReplyTo": "http://localhost:8080/users/1happyturtle/statuses/01F8MHC8VWDRBQR0N1BATDDEM5",
  "interactionPolicy": {
    "canAnnounce": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canLike": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canQuote": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    },
    "canReply": {
      "automaticApproval": [
        "https://www.w3.org/ns/activitystreams#Public"
      ]
    }
  },
  "published": "2024-02-20T12:41:37+02:00",
  "replies": {
    "first": {
      "id": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ/replies?page=true",
      "next": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ/replies?page=true\u0026only_other_accounts=false",
      "partOf": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ/replies",
      "type": "CollectionPage"
    },
    "id": "http://localhost:8080/users/admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ/replies",
    "type": "Collection"
  },
  "replyAuthorization": "http://localhost:8080/users/1happyturtle/authorizations/01J5QVXCCEATJYSXM9H6MZT4JR",
  "tag": {
    "href": "http://localhost:8080/users/1happyturtle",
    "name": "@1happyturtle@localhost:8080",
    "type": "Mention"
  },
  "to": "https://www.w3.org/ns/activitystreams#Public",
  "type": "Note",
  "url": "http://localhost:8080/@admin/statuses/01J5QVB9VC76NPPRQ207GG4DRZ"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestStatusesToASOutboxPage() {
	testAccount := suite.testAccounts["admin_account"]
	ctx := suite.T().Context()

	// get public statuses from testaccount
	statuses, err := suite.db.GetAccountStatuses(ctx, testAccount.ID, 30, true, true, "", "", false, true)
	suite.NoError(err)

	page, err := suite.typeconverter.StatusesToASOutboxPage(ctx, testAccount.OutboxURI, "", "", statuses)
	suite.NoError(err)

	ser, err := ap.Serialize(page)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "http://localhost:8080/users/admin/outbox?page=true",
  "next": "http://localhost:8080/users/admin/outbox?page=true\u0026max_id=01F8MH75CBF9JFX4ZAD54N0W0R",
  "orderedItems": [
    {
      "actor": "http://localhost:8080/users/admin",
      "cc": "http://localhost:8080/users/admin/followers",
      "id": "http://localhost:8080/users/admin/statuses/01F8MHAAY43M6RJ473VQFCVH37/activity#Create",
      "object": "http://localhost:8080/users/admin/statuses/01F8MHAAY43M6RJ473VQFCVH37",
      "published": "2021-10-20T12:36:45Z",
      "to": "https://www.w3.org/ns/activitystreams#Public",
      "type": "Create"
    },
    {
      "actor": "http://localhost:8080/users/admin",
      "cc": "http://localhost:8080/users/admin/followers",
      "id": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R/activity#Create",
      "object": "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R",
      "published": "2021-10-20T11:36:45Z",
      "to": "https://www.w3.org/ns/activitystreams#Public",
      "type": "Create"
    }
  ],
  "partOf": "http://localhost:8080/users/admin/outbox",
  "prev": "http://localhost:8080/users/admin/outbox?page=true\u0026min_id=01F8MHAAY43M6RJ473VQFCVH37",
  "type": "OrderedCollectionPage"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestSelfBoostFollowersOnlyToAS() {
	ctx := suite.T().Context()

	testStatus := suite.testStatuses["local_account_1_status_5"]
	testAccount := suite.testAccounts["local_account_1"]

	boostWrapperStatus, err := suite.typeconverter.StatusToBoost(ctx, testStatus, testAccount, "")
	suite.NoError(err)
	suite.NotNil(boostWrapperStatus)

	// Set some fields to predictable values for the test.
	boostWrapperStatus.ID = "01G74JJ1KS331G2JXHRMZCE0ER"
	boostWrapperStatus.URI = "http://localhost:8080/users/the_mighty_zork/statuses/01G74JJ1KS331G2JXHRMZCE0ER"
	boostWrapperStatus.CreatedAt = testrig.TimeMustParse("2022-06-09T13:12:00Z")

	asBoost, err := suite.typeconverter.BoostToAS(ctx, boostWrapperStatus)
	suite.NoError(err)

	ser, err := ap.Serialize(asBoost)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "actor": "http://localhost:8080/users/the_mighty_zork",
  "cc": "http://localhost:8080/users/the_mighty_zork",
  "id": "http://localhost:8080/users/the_mighty_zork/statuses/01G74JJ1KS331G2JXHRMZCE0ER",
  "object": "http://localhost:8080/users/the_mighty_zork/statuses/01FCTA44PW9H1TB328S9AQXKDS",
  "published": "2022-06-09T13:12:00Z",
  "to": "http://localhost:8080/users/the_mighty_zork/followers",
  "type": "Announce"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestReportToAS() {
	ctx := suite.T().Context()

	testReport := suite.testReports["local_account_2_report_remote_account_1"]
	account := suite.testAccounts["local_account_2"]
	targetAccount := suite.testAccounts["remote_account_1"]
	statuses := []*gtsmodel.Status{suite.testStatuses["remote_account_1_status_1"]}

	testReport.Account = account
	testReport.TargetAccount = targetAccount
	testReport.Statuses = statuses

	flag, err := suite.typeconverter.ReportToASFlag(ctx, testReport)
	suite.NoError(err)

	ser, err := ap.Serialize(flag)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "actor": "http://localhost:8080/users/localhost:8080",
  "content": "dark souls sucks, please yeet this nerd",
  "id": "http://localhost:8080/reports/01GP3AWY4CRDVRNZKW0TEAMB5R",
  "object": [
    "http://fossbros-anonymous.io/users/foss_satan",
    "http://fossbros-anonymous.io/users/foss_satan/statuses/01FVW7JHQFSFK166WWKR8CBA6M"
  ],
  "type": "Flag"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestPinnedStatusesToASSomeItems() {
	ctx := suite.T().Context()

	testAccount := suite.testAccounts["admin_account"]
	statuses, err := suite.db.GetAccountPinnedStatuses(ctx, testAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}

	collection, err := suite.typeconverter.StatusesToASFeaturedCollection(ctx, testAccount.FeaturedCollectionURI, statuses)
	if err != nil {
		suite.FailNow(err.Error())
	}

	ser, err := ap.Serialize(collection)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "http://localhost:8080/users/admin/collections/featured",
  "orderedItems": [
    "http://localhost:8080/users/admin/statuses/01F8MHAAY43M6RJ473VQFCVH37",
    "http://localhost:8080/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R"
  ],
  "totalItems": 2,
  "type": "OrderedCollection"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestPinnedStatusesToASNoItems() {
	ctx := suite.T().Context()

	testAccount := suite.testAccounts["local_account_1"]
	statuses, err := suite.db.GetAccountPinnedStatuses(ctx, testAccount.ID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		suite.FailNow(err.Error())
	}

	collection, err := suite.typeconverter.StatusesToASFeaturedCollection(ctx, testAccount.FeaturedCollectionURI, statuses)
	if err != nil {
		suite.FailNow(err.Error())
	}

	ser, err := ap.Serialize(collection)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "http://localhost:8080/users/the_mighty_zork/collections/featured",
  "orderedItems": [],
  "totalItems": 0,
  "type": "OrderedCollection"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestPinnedStatusesToASOneItem() {
	ctx := suite.T().Context()

	testAccount := suite.testAccounts["local_account_2"]
	statuses, err := suite.db.GetAccountPinnedStatuses(ctx, testAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}

	collection, err := suite.typeconverter.StatusesToASFeaturedCollection(ctx, testAccount.FeaturedCollectionURI, statuses)
	if err != nil {
		suite.FailNow(err.Error())
	}

	ser, err := ap.Serialize(collection)
	suite.NoError(err)

	bytes, err := json.MarshalIndent(ser, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "http://localhost:8080/users/1happyturtle/collections/featured",
  "orderedItems": [
    "http://localhost:8080/users/1happyturtle/statuses/01G20ZM733MGN8J344T4ZDDFY1"
  ],
  "totalItems": 1,
  "type": "OrderedCollection"
}`, string(bytes))
}

func (suite *InternalToASTestSuite) TestPollVoteToASCreate() {
	vote := suite.testPollVotes["remote_account_1_status_2_poll_vote_local_account_1"]

	creates, err := suite.typeconverter.PollVoteToASCreates(suite.T().Context(), vote)
	suite.NoError(err)
	suite.Len(creates, 2)

	createI0, err := ap.Serialize(creates[0])
	suite.NoError(err)

	createI1, err := ap.Serialize(creates[1])
	suite.NoError(err)

	bytes0, err := json.MarshalIndent(createI0, "", "  ")
	suite.NoError(err)

	bytes1, err := json.MarshalIndent(createI1, "", "  ")
	suite.NoError(err)

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "actor": "http://localhost:8080/users/the_mighty_zork",
  "id": "http://localhost:8080/users/the_mighty_zork/activity#vote0/http://fossbros-anonymous.io/users/foss_satan/statuses/01HEN2QRFA8H3C6QPN7RD4KSR6",
  "object": {
    "attributedTo": "http://localhost:8080/users/the_mighty_zork",
    "id": "http://localhost:8080/users/the_mighty_zork#01HEN2R65468ZG657C4ZPHJ4EX/votes/1",
    "inReplyTo": "http://fossbros-anonymous.io/users/foss_satan/statuses/01HEN2QRFA8H3C6QPN7RD4KSR6",
    "name": "tissues",
    "to": "http://fossbros-anonymous.io/users/foss_satan",
    "type": "Note"
  },
  "published": "2021-09-11T11:45:37+02:00",
  "to": "http://fossbros-anonymous.io/users/foss_satan",
  "type": "Create"
}`, string(bytes0))

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "actor": "http://localhost:8080/users/the_mighty_zork",
  "id": "http://localhost:8080/users/the_mighty_zork/activity#vote1/http://fossbros-anonymous.io/users/foss_satan/statuses/01HEN2QRFA8H3C6QPN7RD4KSR6",
  "object": {
    "attributedTo": "http://localhost:8080/users/the_mighty_zork",
    "id": "http://localhost:8080/users/the_mighty_zork#01HEN2R65468ZG657C4ZPHJ4EX/votes/2",
    "inReplyTo": "http://fossbros-anonymous.io/users/foss_satan/statuses/01HEN2QRFA8H3C6QPN7RD4KSR6",
    "name": "financial times",
    "to": "http://fossbros-anonymous.io/users/foss_satan",
    "type": "Note"
  },
  "published": "2021-09-11T11:45:37+02:00",
  "to": "http://fossbros-anonymous.io/users/foss_satan",
  "type": "Create"
}`, string(bytes1))
}

func (suite *InternalToASTestSuite) TestImpoliteInteractionReqToASAcceptAnnounce() {
	acceptingAccount := suite.testAccounts["local_account_1"]
	interactingAccount := suite.testAccounts["remote_account_1"]

	req := &gtsmodel.InteractionRequest{
		ID:                    "01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		TargetStatusID:        "01JJYCVKCXB9JTQD1XW2KB8MT3",
		TargetStatus:          &gtsmodel.Status{URI: "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3"},
		TargetAccountID:       acceptingAccount.ID,
		TargetAccount:         acceptingAccount,
		InteractingAccountID:  interactingAccount.ID,
		InteractingAccount:    interactingAccount,
		InteractionRequestURI: "https://fossbros-anonymous.io/users/foss_satan/statuses/01J1AKRRHQ6MDDQHV0TP716T2K" + gtsmodel.ImpoliteAnnounceRequestSuffix,
		InteractionURI:        "https://fossbros-anonymous.io/users/foss_satan/statuses/01J1AKRRHQ6MDDQHV0TP716T2K",
		InteractionType:       gtsmodel.InteractionAnnounce,
		Polite:                util.Ptr(false),
		ResponseURI:           "http://localhost:8080/users/the_mighty_zork/accepts/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		AcceptedAt:            testrig.TimeMustParse("2022-06-09T13:12:00Z"),
	}

	accept, err := suite.typeconverter.InteractionReqToASAccept(
		suite.T().Context(),
		req,
	)
	if err != nil {
		suite.FailNow(err.Error())
	}

	i, err := ap.Serialize(accept)
	if err != nil {
		suite.FailNow(err.Error())
	}

	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		suite.FailNow(err.Error())
	}

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "actor": "http://localhost:8080/users/the_mighty_zork",
  "cc": [
    "https://www.w3.org/ns/activitystreams#Public",
    "http://localhost:8080/users/the_mighty_zork/followers"
  ],
  "id": "http://localhost:8080/users/the_mighty_zork/accepts/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
  "object": "https://fossbros-anonymous.io/users/foss_satan/statuses/01J1AKRRHQ6MDDQHV0TP716T2K",
  "target": "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3",
  "to": "http://fossbros-anonymous.io/users/foss_satan",
  "type": "Accept"
}`, string(b))
}

func (suite *InternalToASTestSuite) TestImpoliteInteractionReqToASAcceptLike() {
	acceptingAccount := suite.testAccounts["local_account_1"]
	interactingAccount := suite.testAccounts["remote_account_1"]

	req := &gtsmodel.InteractionRequest{
		ID:                    "01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		TargetStatusID:        "01JJYCVKCXB9JTQD1XW2KB8MT3",
		TargetStatus:          &gtsmodel.Status{URI: "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3"},
		TargetAccountID:       acceptingAccount.ID,
		TargetAccount:         acceptingAccount,
		InteractingAccountID:  interactingAccount.ID,
		InteractingAccount:    interactingAccount,
		InteractionRequestURI: "https://fossbros-anonymous.io/users/foss_satan/likes/01J1AKRRHQ6MDDQHV0TP716T2K" + gtsmodel.ImpoliteLikeRequestSuffix,
		InteractionURI:        "https://fossbros-anonymous.io/users/foss_satan/likes/01J1AKRRHQ6MDDQHV0TP716T2K",
		InteractionType:       gtsmodel.InteractionLike,
		Polite:                util.Ptr(false),
		ResponseURI:           "http://localhost:8080/users/the_mighty_zork/accepts/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		AcceptedAt:            testrig.TimeMustParse("2022-06-09T13:12:00Z"),
	}

	accept, err := suite.typeconverter.InteractionReqToASAccept(
		suite.T().Context(),
		req,
	)
	if err != nil {
		suite.FailNow(err.Error())
	}

	i, err := ap.Serialize(accept)
	if err != nil {
		suite.FailNow(err.Error())
	}

	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		suite.FailNow(err.Error())
	}

	suite.Equal(`{
  "@context": "https://www.w3.org/ns/activitystreams",
  "actor": "http://localhost:8080/users/the_mighty_zork",
  "id": "http://localhost:8080/users/the_mighty_zork/accepts/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
  "object": "https://fossbros-anonymous.io/users/foss_satan/likes/01J1AKRRHQ6MDDQHV0TP716T2K",
  "target": "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3",
  "to": "http://fossbros-anonymous.io/users/foss_satan",
  "type": "Accept"
}`, string(b))
}

func (suite *InternalToASTestSuite) TestInteractionReqToASAcceptLikePolite() {
	acceptingAccount := suite.testAccounts["local_account_1"]
	interactingAccount := suite.testAccounts["remote_account_1"]

	req := &gtsmodel.InteractionRequest{
		ID:                    "01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		TargetStatusID:        "01JJYCVKCXB9JTQD1XW2KB8MT3",
		TargetStatus:          &gtsmodel.Status{URI: "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3"},
		TargetAccountID:       acceptingAccount.ID,
		TargetAccount:         acceptingAccount,
		InteractingAccountID:  interactingAccount.ID,
		InteractingAccount:    interactingAccount,
		InteractionRequestURI: "https://fossbros-anonymous.io/users/foss_satan/interaction_requests/01J1AKRRHQ6MDDQHV0TP716T2K",
		InteractionURI:        "https://fossbros-anonymous.io/users/foss_satan/likes/01J1AKRRHQ6MDDQHV0TP716T2K",
		InteractionType:       gtsmodel.InteractionLike,
		Polite:                util.Ptr(true),
		ResponseURI:           "http://localhost:8080/users/the_mighty_zork/accepts/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		AuthorizationURI:      "http://localhost:8080/users/the_mighty_zork/authorizations/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		AcceptedAt:            testrig.TimeMustParse("2022-06-09T13:12:00Z"),
	}

	accept, err := suite.typeconverter.InteractionReqToASAccept(
		suite.T().Context(),
		req,
	)
	if err != nil {
		suite.FailNow(err.Error())
	}

	i, err := ap.Serialize(accept)
	if err != nil {
		suite.FailNow(err.Error())
	}

	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		suite.FailNow(err.Error())
	}

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://www.w3.org/ns/activitystreams"
  ],
  "actor": "http://localhost:8080/users/the_mighty_zork",
  "id": "http://localhost:8080/users/the_mighty_zork/accepts/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
  "object": {
    "actor": "http://fossbros-anonymous.io/users/foss_satan",
    "id": "https://fossbros-anonymous.io/users/foss_satan/interaction_requests/01J1AKRRHQ6MDDQHV0TP716T2K",
    "instrument": "https://fossbros-anonymous.io/users/foss_satan/likes/01J1AKRRHQ6MDDQHV0TP716T2K",
    "object": "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3",
    "type": "LikeRequest"
  },
  "result": "http://localhost:8080/users/the_mighty_zork/authorizations/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
  "to": "http://fossbros-anonymous.io/users/foss_satan",
  "type": "Accept"
}`, string(b))
}

func (suite *InternalToASTestSuite) TestPoliteInteractionReqToASAuthorization() {
	acceptingAccount := suite.testAccounts["local_account_1"]
	interactingAccount := suite.testAccounts["remote_account_1"]

	req := &gtsmodel.InteractionRequest{
		ID:                    "01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		TargetStatusID:        "01JJYCVKCXB9JTQD1XW2KB8MT3",
		TargetStatus:          &gtsmodel.Status{URI: "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3"},
		TargetAccountID:       acceptingAccount.ID,
		TargetAccount:         acceptingAccount,
		InteractingAccountID:  interactingAccount.ID,
		InteractingAccount:    interactingAccount,
		InteractionURI:        "https://fossbros-anonymous.io/users/foss_satan/likes/01J1AKRRHQ6MDDQHV0TP716T2K",
		InteractionType:       gtsmodel.InteractionLike,
		Polite:                util.Ptr(true),
		InteractionRequestURI: "https://fossbros-anonymous.io/users/foss_satan/interaction_requests/01J1AKRRHQ6MDDQHV0TP716T2K",
		ResponseURI:           "http://localhost:8080/users/the_mighty_zork/accepts/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		AuthorizationURI:      "http://localhost:8080/users/the_mighty_zork/authorizations/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		AcceptedAt:            testrig.TimeMustParse("2022-06-09T13:12:00Z"),
	}

	auth, err := suite.typeconverter.InteractionReqToASAuthorization(
		suite.T().Context(),
		req,
	)
	if err != nil {
		suite.FailNow(err.Error())
	}

	i, err := ap.Serialize(auth)
	if err != nil {
		suite.FailNow(err.Error())
	}

	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		suite.FailNow(err.Error())
	}

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://www.w3.org/ns/activitystreams"
  ],
  "attributedTo": "http://localhost:8080/users/the_mighty_zork",
  "id": "http://localhost:8080/users/the_mighty_zork/authorizations/01J1AKMZ8JE5NW0ZSFTRC1JJNE",
  "interactingObject": "https://fossbros-anonymous.io/users/foss_satan/likes/01J1AKRRHQ6MDDQHV0TP716T2K",
  "interactionTarget": "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3",
  "type": "LikeAuthorization"
}`, string(b))
}

func (suite *InternalToASTestSuite) TestInteractionReqToASInteractionRequestable() {
	targetAccount := suite.testAccounts["local_account_1"]
	interactingAccount := suite.testAccounts["remote_account_1"]
	interactingStatus := suite.testStatuses["remote_account_1_status_1"]

	req := &gtsmodel.InteractionRequest{
		ID:                    "01J1AKMZ8JE5NW0ZSFTRC1JJNE",
		TargetStatusID:        "01JJYCVKCXB9JTQD1XW2KB8MT3",
		TargetStatus:          &gtsmodel.Status{URI: "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3"},
		TargetAccountID:       targetAccount.ID,
		TargetAccount:         targetAccount,
		InteractingAccountID:  interactingAccount.ID,
		InteractingAccount:    interactingAccount,
		InteractionRequestURI: "https://fossbros-anonymous.io/users/foss_satan/interaction_requests/01J1AKRRHQ6MDDQHV0TP716T2K",
		InteractionURI:        "https://fossbros-anonymous.io/users/foss_satan/statuses/01J1AKRRHQ6MDDQHV0TP716T2K",
		InteractionType:       gtsmodel.InteractionReply,
		Reply:                 interactingStatus,
		Polite:                util.Ptr(true),
	}

	accept, err := suite.typeconverter.InteractionReqToASInteractionRequestable(
		suite.T().Context(),
		req,
	)
	if err != nil {
		suite.FailNow(err.Error())
	}

	i, err := ap.Serialize(accept)
	if err != nil {
		suite.FailNow(err.Error())
	}

	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		suite.FailNow(err.Error())
	}

	suite.Equal(`{
  "@context": [
    "https://gotosocial.org/ns",
    "https://www.w3.org/ns/activitystreams",
    {
      "blurhash": "toot:blurhash",
      "toot": "http://joinmastodon.org/ns#"
    }
  ],
  "actor": "http://fossbros-anonymous.io/users/foss_satan",
  "id": "https://fossbros-anonymous.io/users/foss_satan/interaction_requests/01J1AKRRHQ6MDDQHV0TP716T2K",
  "instrument": {
    "attachment": [
      {
        "blurhash": "L3Q9_@4n9E?axW4mD$Mx~q00Di%L",
        "mediaType": "image/jpeg",
        "name": "tweet from thoughts of dog: i drank. all the water. in my bowl. earlier. but just now. i returned. to the same bowl. and it was. full again.. the bowl. is haunted",
        "type": "Image",
        "url": "http://localhost:8080/fileserver/01F8MH5ZK5VRH73AKHQM6Y9VNX/attachment/original/01FVW7RXPQ8YJHTEXYPE7Q8ZY0.jpg"
      }
    ],
    "attributedTo": "http://fossbros-anonymous.io/users/foss_satan",
    "cc": "https://www.w3.org/ns/activitystreams#Public",
    "content": "\u003cp\u003edark souls status bot: \"thoughts of dog\"\u003c/p\u003e",
    "contentMap": {
      "en": "\u003cp\u003edark souls status bot: \"thoughts of dog\"\u003c/p\u003e"
    },
    "id": "http://fossbros-anonymous.io/users/foss_satan/statuses/01FVW7JHQFSFK166WWKR8CBA6M",
    "interactionPolicy": {
      "canAnnounce": {
        "automaticApproval": [
          "https://www.w3.org/ns/activitystreams#Public"
        ]
      },
      "canLike": {
        "automaticApproval": [
          "https://www.w3.org/ns/activitystreams#Public"
        ]
      },
      "canQuote": {
        "automaticApproval": [
          "https://www.w3.org/ns/activitystreams#Public"
        ]
      },
      "canReply": {
        "automaticApproval": [
          "https://www.w3.org/ns/activitystreams#Public"
        ]
      }
    },
    "published": "2021-09-20T12:40:37+02:00",
    "replies": {
      "first": {
        "id": "http://fossbros-anonymous.io/users/foss_satan/statuses/01FVW7JHQFSFK166WWKR8CBA6M/replies?page=true",
        "next": "http://fossbros-anonymous.io/users/foss_satan/statuses/01FVW7JHQFSFK166WWKR8CBA6M/replies?page=true\u0026only_other_accounts=false",
        "partOf": "http://fossbros-anonymous.io/users/foss_satan/statuses/01FVW7JHQFSFK166WWKR8CBA6M/replies",
        "type": "CollectionPage"
      },
      "id": "http://fossbros-anonymous.io/users/foss_satan/statuses/01FVW7JHQFSFK166WWKR8CBA6M/replies",
      "type": "Collection"
    },
    "to": "http://fossbros-anonymous.io/users/foss_satan/followers",
    "type": "Note",
    "url": "http://fossbros-anonymous.io/@foss_satan/statuses/01FVW7JHQFSFK166WWKR8CBA6M"
  },
  "object": "http://localhost:8080/users/the_mighty_zork/statuses/01JJYCVKCXB9JTQD1XW2KB8MT3",
  "to": "http://localhost:8080/users/the_mighty_zork",
  "type": "ReplyRequest"
}`, string(b))
}

func (suite *InternalToASTestSuite) TestQuoteReqToASInteractionRequestable() {
	interactingAccount := suite.testAccounts["local_account_1"]
	targetAccount := suite.testAccounts["remote_account_1"]
	quotingStatus := new(gtsmodel.Status)
	*quotingStatus = *suite.testStatuses["local_account_1_status_1"]
	targetStatus := suite.testStatuses["remote_account_1_status_1"]

	req := &gtsmodel.InteractionRequest{
		ID:                    "01J1AKMZ8JE5NW0ZSFTRC1JJNQ",
		TargetStatusID:        targetStatus.ID,
		TargetStatus:          targetStatus,
		TargetAccountID:       targetAccount.ID,
		TargetAccount:         targetAccount,
		InteractingAccountID:  interactingAccount.ID,
		InteractingAccount:    interactingAccount,
		InteractionRequestURI: "http://localhost:8080/users/the_mighty_zork/quote_requests/01J1AKMZ8JE5NW0ZSFTRC1JJNQ",
		InteractionURI:        quotingStatus.URI,
		InteractionType:       gtsmodel.InteractionQuote,
		Quote:                 quotingStatus,
		Polite:                util.Ptr(true),
	}

	request, err := suite.typeconverter.InteractionReqToASInteractionRequestable(
		suite.T().Context(),
		req,
	)
	suite.NoError(err)
	suite.Equal(ap.ActivityQuoteRequest, request.GetTypeName())
	suite.Equal(req.InteractionRequestURI, ap.GetJSONLDId(request).String())
	objectIRI, err := ap.GetOneObjectIRI(request)
	suite.NoError(err)
	suite.Equal(targetStatus.URI, objectIRI.String())

	instruments := ap.ExtractInstruments(request)
	suite.Len(instruments, 1)
	suite.Equal(quotingStatus.URI, ap.GetJSONLDId(instruments[0].GetType()).String())
}

func TestInternalToASTestSuite(t *testing.T) {
	suite.Run(t, new(InternalToASTestSuite))
}
