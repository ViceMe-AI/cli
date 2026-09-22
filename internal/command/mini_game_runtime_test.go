package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The SDK and DOM below are external host boundaries. The installed configuration,
// offline runtime, card construction, HMAC verification and storage are real.
func TestMiniGameInstalledRuntimeRedeemsAndRestoresCode(t *testing.T) {
	manifest := miniGameFixture()
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
		"sharedSecret": manifest.SharedSecret,
		"workId":       manifest.WorkID, "itemId": manifest.Items[0].ID, "alias": manifest.Items[0].Alias,
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
import {createHash, createHmac, webcrypto} from 'node:crypto';
import {join} from 'node:path';
const input = JSON.parse(readFileSync(0, 'utf8'));
const project = process.argv[1];
const runtime = readFileSync(join(project, 'viceme/mini-game-commerce.js'), 'utf8');
const configuration = readFileSync(join(project, 'viceme/mini-game-config.js'), 'utf8');
const store = new Map();
let now = 1800000000000;
class GameDate extends Date { static now() { return now; } }
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
 const context = createContext({window, URL, console, Date: GameDate});
 runInContext(runtime,context);
 runInContext(configuration,context);
 return context.ViceMeMiniGame ?? context.window.ViceMeMiniGame;
}
const game = load(store);
assert.deepEqual(JSON.parse(JSON.stringify(await game.initialize())),{ok:true});
assert.equal((await game.isUnlocked(input.alias)).unlocked,false);
const card = await game.createPurchaseCard(input.alias);
assert.equal(card.ok,true);
assert.equal(saved,1);
assert.ok(cardText.includes('SANDBOX TEST ONLY'));
const checkout = new URL(card.checkoutUrl);
assert.equal(checkout.pathname,'/checkout/mini-game');
assert.equal(checkout.searchParams.get('itemId'),input.itemId);
assert.equal(checkout.searchParams.get('runtimeVersion'),'2.0.0');
assert.equal(checkout.searchParams.has('price'),false);
const uuid = value => Buffer.from(value.replaceAll('-',''),'hex');
function codeFor(installationId, window) {
 const installationDigest = createHash('sha256').update('ViceMe.MiniGame.Installation.v1\0').update(uuid(installationId)).digest().subarray(0,16);
 const counter = Buffer.alloc(8);
 counter.writeBigUInt64BE(BigInt(window));
 const message = Buffer.concat([Buffer.from('ViceMe.MiniGame.UnlockCode.v2\0'),uuid(input.workId),Buffer.from([0x10]),installationDigest,uuid(input.itemId),counter]);
 const digest = createHmac('sha256',Buffer.from(input.sharedSecret,'hex')).update(message).digest();
 return ((digest.readUInt32BE(digest[31] & 15) & 0x7fffffff) % 1000000).toString().padStart(6,'0');
}
const installationId = checkout.searchParams.get('installationId');
assert.equal(cardText.some(text => text.includes(installationId)),false);
assert.equal(cardText.some(text => text.includes(input.sharedSecret)),false);
const other = load(new Map());
assert.deepEqual(JSON.parse(JSON.stringify(await other.initialize())),{ok:true});
const otherCard = await other.createPurchaseCard(input.alias);
assert.equal(otherCard.ok,true);
const otherInstallationId = new URL(otherCard.checkoutUrl).searchParams.get('installationId');
assert.notEqual(otherInstallationId,installationId);
// Find a leading-zero fixture that cannot collide with another installation's
// three accepted windows. Six-digit codes deliberately have a finite keyspace.
let window = Math.floor(now / 30000);
let code;
for (let attempt = 0; attempt < 10000; attempt++, window++) {
 const candidate = codeFor(installationId,window);
 if (candidate.startsWith('0') && [-1,0,1].every(offset => codeFor(otherInstallationId,window + offset) !== candidate)) {
  code = candidate;
  break;
 }
}
assert.match(code,/^0[0-9]{5}$/);
now = window * 30000 + 15000;
assert.equal((await game.redeemCode(input.alias,code.slice(1))).ok,false);
assert.equal((await game.isUnlocked(input.alias)).unlocked,false);
assert.equal((await other.redeemCode(input.alias,code)).ok,false);
assert.equal((await other.isUnlocked(input.alias)).unlocked,false);
assert.deepEqual(JSON.parse(JSON.stringify(await game.redeemCode(input.alias,code))),{ok:true,unlocked:true});
assert.equal((await game.isUnlocked(input.alias)).unlocked,true);
now += 300000;
const restarted = load(store);
assert.deepEqual(JSON.parse(JSON.stringify(await restarted.initialize())),{ok:true});
assert.equal((await restarted.isUnlocked(input.alias)).unlocked,true);
const restartedCard = await restarted.createPurchaseCard(input.alias);
assert.equal(restartedCard.ok,true);
assert.equal(new URL(restartedCard.checkoutUrl).searchParams.get('installationId'),installationId);
console.log('card, redemption, restart and wrong-installation rejection verified');
`
