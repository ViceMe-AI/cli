package command

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The SDK and DOM below are external host boundaries. The installed configuration,
// offline runtime, card construction, signature verification and storage are real.
func TestMiniGameInstalledRuntimeRedeemsAndRestoresLicense(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	manifest := miniGameFixture()
	manifest.PublicKey = base64.RawURLEncoding.EncodeToString(publicKey)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/v1/cli/merchant/works/"+manifest.WorkID+"/mini-game-integration" {
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()
	project := t.TempDir()
	writeMiniGameHost(t, project)
	unrelated := filepath.Join(project, "original-game-data.json")
	original := []byte("{\"score\":42,\"levels\":[1,2,3]}\n")
	if err := os.WriteFile(unrelated, original, 0600); err != nil {
		t.Fatal(err)
	}
	exit, output := executeMerchantEngagementCommand(t, server, []string{
		"mini-game", "integrate", "--work", manifest.WorkID,
		"--merchant-account", merchantEngagementMerchantID, "--environment", "sandbox", "--project", project,
	})
	if exit != 0 {
		t.Fatalf("integration failed: %s", output)
	}
	input, err := json.Marshal(map[string]string{
		"privateKey": base64.StdEncoding.EncodeToString(privateDER),
		"workId":     manifest.WorkID, "itemId": manifest.Items[0].ID, "alias": manifest.Items[0].Alias,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("node", "--input-type=module", "-e", miniGameRuntimeScenario, project)
	command.Stdin = bytes.NewReader(input)
	result, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("installed runtime scenario failed: %v\n%s", err, result)
	}
	if string(result) != "card, redemption, restart and wrong-installation rejection verified\n" {
		t.Fatalf("unexpected runtime result: %s", result)
	}
	actual, err := os.ReadFile(unrelated)
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatalf("unrelated content changed: %v", err)
	}
}

const miniGameRuntimeScenario = `
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createContext, runInContext} from 'node:vm';
import {createHash, createPrivateKey, randomBytes, sign, webcrypto} from 'node:crypto';
import {join} from 'node:path';
const input = JSON.parse(readFileSync(0, 'utf8'));
const project = process.argv[1];
const runtime = readFileSync(join(project, 'viceme/mini-game-commerce.js'), 'utf8');
const configuration = readFileSync(join(project, 'viceme/mini-game-config.js'), 'utf8');
const store = new Map();
let saved = 0;
const cardText = [];
const context2d = {
 fillRect() {}, fillText(value) { cardText.push(value); },
 measureText(value) { return {width: value.length * 12}; },
};
function load(storage) {
 const window = {
  crypto: webcrypto,
  document: {createElement(name) {
   assert.equal(name, 'canvas');
   return {getContext: () => context2d, toDataURL: () => 'data:image/png;base64,host-canvas', width:0,height:0};
  }},
  xhs: {launchOptions: {miniToolEnv: {buildVersion: 9460000}}, miniTool: {
   async getStorageInfo() {return {keys:[...storage.keys()]};},
   async getStorage({key,encrypt}) {assert.equal(encrypt,true); return {data:structuredClone(storage.get(key))};},
   async setStorage({key,data,encrypt}) {assert.equal(encrypt,true); storage.set(key,structuredClone(data));},
   async writeTempFile({data}) {assert.match(data,/^data:image\/png/);return {filePath:'wxfile://card.png'};},
   async saveImageToPhotosAlbum({filePath}) {assert.equal(filePath,'wxfile://card.png');saved++;},
  }},
 };
 const context = createContext({window, URL, console});
 runInContext(runtime,context);
 runInContext(configuration,context);
 return context.ViceMeMiniGame ?? context.window.ViceMeMiniGame;
}
const game = load(store);
assert.equal((await game.initialize()).ok,true);
assert.equal((await game.isUnlocked(input.alias)).unlocked,false);
const card = await game.createPurchaseCard(input.alias);
assert.equal(card.ok,true);
assert.equal(saved,1);
assert.ok(cardText.includes('SANDBOX TEST ONLY'));
const checkout = new URL(card.checkoutUrl);
assert.equal(checkout.pathname,'/checkout/mini-game');
assert.equal(checkout.searchParams.get('itemId'),input.itemId);
assert.equal(checkout.searchParams.get('runtimeVersion'),'1.0.0');
assert.equal(checkout.searchParams.has('price'),false);
const uuid = value => Buffer.from(value.replaceAll('-',''),'hex');
const digest = createHash('sha256').update('ViceMe.MiniGame.Installation.v1\0').update(uuid(checkout.searchParams.get('installationId'))).digest().subarray(0,16);
const payload = Buffer.concat([Buffer.from([0x10]),uuid(input.itemId),digest,randomBytes(8)]);
const message = Buffer.concat([Buffer.from('ViceMe.MiniGame.License.v1\0'),uuid(input.workId),payload]);
const key = createPrivateKey({key:Buffer.from(input.privateKey,'base64'),format:'der',type:'pkcs8'});
const token = Buffer.concat([payload,sign(null,message,key)]).toString('base64url');
assert.equal(token.length,140);
assert.equal((await game.redeemLicense(input.alias,token)).ok,true);
assert.equal((await game.isUnlocked(input.alias)).unlocked,true);
const restarted = load(store);
assert.equal((await restarted.initialize()).ok,true);
assert.equal((await restarted.isUnlocked(input.alias)).unlocked,true);
const other = load(new Map());
assert.equal((await other.initialize()).ok,true);
assert.equal((await other.redeemLicense(input.alias,token)).ok,false);
assert.equal((await other.isUnlocked(input.alias)).unlocked,false);
console.log('card, redemption, restart and wrong-installation rejection verified');
`
