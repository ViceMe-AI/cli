"use strict";
var ViceMeMiniGameCommerce = (() => {
  var __create = Object.create;
  var __defProp = Object.defineProperty, __defProps = Object.defineProperties, __getOwnPropDesc = Object.getOwnPropertyDescriptor, __getOwnPropDescs = Object.getOwnPropertyDescriptors, __getOwnPropNames = Object.getOwnPropertyNames, __getOwnPropSymbols = Object.getOwnPropertySymbols, __getProtoOf = Object.getPrototypeOf, __hasOwnProp = Object.prototype.hasOwnProperty, __propIsEnum = Object.prototype.propertyIsEnumerable;
  var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: !0, configurable: !0, writable: !0, value }) : obj[key] = value, __spreadValues = (a, b) => {
    for (var prop in b || (b = {}))
      __hasOwnProp.call(b, prop) && __defNormalProp(a, prop, b[prop]);
    if (__getOwnPropSymbols)
      for (var prop of __getOwnPropSymbols(b))
        __propIsEnum.call(b, prop) && __defNormalProp(a, prop, b[prop]);
    return a;
  }, __spreadProps = (a, b) => __defProps(a, __getOwnPropDescs(b));
  var __commonJS = (cb, mod) => function() {
    return mod || (0, cb[__getOwnPropNames(cb)[0]])((mod = { exports: {} }).exports, mod), mod.exports;
  };
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, { get: all[name], enumerable: !0 });
  }, __copyProps = (to, from, except, desc) => {
    if (from && typeof from == "object" || typeof from == "function")
      for (let key of __getOwnPropNames(from))
        !__hasOwnProp.call(to, key) && key !== except && __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
    return to;
  };
  var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(
    // If the importer is in node compatibility mode or this is not an ESM
    // file that has been converted to a CommonJS file using a Babel-
    // compatible transform (i.e. "__esModule" has not been set), then set
    // "default" to the CommonJS "module.exports" for node compatibility.
    isNodeMode || !mod || !mod.__esModule ? __defProp(target, "default", { value: mod, enumerable: !0 }) : target,
    mod
  )), __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: !0 }), mod);

  // ../../node_modules/.pnpm/@stablelib+int@1.0.1/node_modules/@stablelib/int/lib/int.js
  var require_int = __commonJS({
    "../../node_modules/.pnpm/@stablelib+int@1.0.1/node_modules/@stablelib/int/lib/int.js"(exports) {
      "use strict";
      Object.defineProperty(exports, "__esModule", { value: !0 });
      function imulShim(a, b) {
        var ah = a >>> 16 & 65535, al = a & 65535, bh = b >>> 16 & 65535, bl = b & 65535;
        return al * bl + (ah * bl + al * bh << 16 >>> 0) | 0;
      }
      exports.mul = Math.imul || imulShim;
      function add(a, b) {
        return a + b | 0;
      }
      exports.add = add;
      function sub(a, b) {
        return a - b | 0;
      }
      exports.sub = sub;
      function rotl(x, n) {
        return x << n | x >>> 32 - n;
      }
      exports.rotl = rotl;
      function rotr(x, n) {
        return x << 32 - n | x >>> n;
      }
      exports.rotr = rotr;
      function isIntegerShim(n) {
        return typeof n == "number" && isFinite(n) && Math.floor(n) === n;
      }
      exports.isInteger = Number.isInteger || isIntegerShim;
      exports.MAX_SAFE_INTEGER = 9007199254740991;
      exports.isSafeInteger = function(n) {
        return exports.isInteger(n) && n >= -exports.MAX_SAFE_INTEGER && n <= exports.MAX_SAFE_INTEGER;
      };
    }
  });

  // ../../node_modules/.pnpm/@stablelib+binary@1.0.1/node_modules/@stablelib/binary/lib/binary.js
  var require_binary = __commonJS({
    "../../node_modules/.pnpm/@stablelib+binary@1.0.1/node_modules/@stablelib/binary/lib/binary.js"(exports) {
      "use strict";
      Object.defineProperty(exports, "__esModule", { value: !0 });
      var int_1 = require_int();
      function readInt16BE(array, offset) {
        return offset === void 0 && (offset = 0), (array[offset + 0] << 8 | array[offset + 1]) << 16 >> 16;
      }
      exports.readInt16BE = readInt16BE;
      function readUint16BE(array, offset) {
        return offset === void 0 && (offset = 0), (array[offset + 0] << 8 | array[offset + 1]) >>> 0;
      }
      exports.readUint16BE = readUint16BE;
      function readInt16LE(array, offset) {
        return offset === void 0 && (offset = 0), (array[offset + 1] << 8 | array[offset]) << 16 >> 16;
      }
      exports.readInt16LE = readInt16LE;
      function readUint16LE(array, offset) {
        return offset === void 0 && (offset = 0), (array[offset + 1] << 8 | array[offset]) >>> 0;
      }
      exports.readUint16LE = readUint16LE;
      function writeUint16BE(value, out, offset) {
        return out === void 0 && (out = new Uint8Array(2)), offset === void 0 && (offset = 0), out[offset + 0] = value >>> 8, out[offset + 1] = value >>> 0, out;
      }
      exports.writeUint16BE = writeUint16BE;
      exports.writeInt16BE = writeUint16BE;
      function writeUint16LE(value, out, offset) {
        return out === void 0 && (out = new Uint8Array(2)), offset === void 0 && (offset = 0), out[offset + 0] = value >>> 0, out[offset + 1] = value >>> 8, out;
      }
      exports.writeUint16LE = writeUint16LE;
      exports.writeInt16LE = writeUint16LE;
      function readInt32BE(array, offset) {
        return offset === void 0 && (offset = 0), array[offset] << 24 | array[offset + 1] << 16 | array[offset + 2] << 8 | array[offset + 3];
      }
      exports.readInt32BE = readInt32BE;
      function readUint32BE(array, offset) {
        return offset === void 0 && (offset = 0), (array[offset] << 24 | array[offset + 1] << 16 | array[offset + 2] << 8 | array[offset + 3]) >>> 0;
      }
      exports.readUint32BE = readUint32BE;
      function readInt32LE(array, offset) {
        return offset === void 0 && (offset = 0), array[offset + 3] << 24 | array[offset + 2] << 16 | array[offset + 1] << 8 | array[offset];
      }
      exports.readInt32LE = readInt32LE;
      function readUint32LE(array, offset) {
        return offset === void 0 && (offset = 0), (array[offset + 3] << 24 | array[offset + 2] << 16 | array[offset + 1] << 8 | array[offset]) >>> 0;
      }
      exports.readUint32LE = readUint32LE;
      function writeUint32BE(value, out, offset) {
        return out === void 0 && (out = new Uint8Array(4)), offset === void 0 && (offset = 0), out[offset + 0] = value >>> 24, out[offset + 1] = value >>> 16, out[offset + 2] = value >>> 8, out[offset + 3] = value >>> 0, out;
      }
      exports.writeUint32BE = writeUint32BE;
      exports.writeInt32BE = writeUint32BE;
      function writeUint32LE(value, out, offset) {
        return out === void 0 && (out = new Uint8Array(4)), offset === void 0 && (offset = 0), out[offset + 0] = value >>> 0, out[offset + 1] = value >>> 8, out[offset + 2] = value >>> 16, out[offset + 3] = value >>> 24, out;
      }
      exports.writeUint32LE = writeUint32LE;
      exports.writeInt32LE = writeUint32LE;
      function readInt64BE(array, offset) {
        offset === void 0 && (offset = 0);
        var hi = readInt32BE(array, offset), lo = readInt32BE(array, offset + 4);
        return hi * 4294967296 + lo - (lo >> 31) * 4294967296;
      }
      exports.readInt64BE = readInt64BE;
      function readUint64BE(array, offset) {
        offset === void 0 && (offset = 0);
        var hi = readUint32BE(array, offset), lo = readUint32BE(array, offset + 4);
        return hi * 4294967296 + lo;
      }
      exports.readUint64BE = readUint64BE;
      function readInt64LE(array, offset) {
        offset === void 0 && (offset = 0);
        var lo = readInt32LE(array, offset), hi = readInt32LE(array, offset + 4);
        return hi * 4294967296 + lo - (lo >> 31) * 4294967296;
      }
      exports.readInt64LE = readInt64LE;
      function readUint64LE(array, offset) {
        offset === void 0 && (offset = 0);
        var lo = readUint32LE(array, offset), hi = readUint32LE(array, offset + 4);
        return hi * 4294967296 + lo;
      }
      exports.readUint64LE = readUint64LE;
      function writeUint64BE(value, out, offset) {
        return out === void 0 && (out = new Uint8Array(8)), offset === void 0 && (offset = 0), writeUint32BE(value / 4294967296 >>> 0, out, offset), writeUint32BE(value >>> 0, out, offset + 4), out;
      }
      exports.writeUint64BE = writeUint64BE;
      exports.writeInt64BE = writeUint64BE;
      function writeUint64LE(value, out, offset) {
        return out === void 0 && (out = new Uint8Array(8)), offset === void 0 && (offset = 0), writeUint32LE(value >>> 0, out, offset), writeUint32LE(value / 4294967296 >>> 0, out, offset + 4), out;
      }
      exports.writeUint64LE = writeUint64LE;
      exports.writeInt64LE = writeUint64LE;
      function readUintBE(bitLength, array, offset) {
        if (offset === void 0 && (offset = 0), bitLength % 8 !== 0)
          throw new Error("readUintBE supports only bitLengths divisible by 8");
        if (bitLength / 8 > array.length - offset)
          throw new Error("readUintBE: array is too short for the given bitLength");
        for (var result = 0, mul = 1, i = bitLength / 8 + offset - 1; i >= offset; i--)
          result += array[i] * mul, mul *= 256;
        return result;
      }
      exports.readUintBE = readUintBE;
      function readUintLE(bitLength, array, offset) {
        if (offset === void 0 && (offset = 0), bitLength % 8 !== 0)
          throw new Error("readUintLE supports only bitLengths divisible by 8");
        if (bitLength / 8 > array.length - offset)
          throw new Error("readUintLE: array is too short for the given bitLength");
        for (var result = 0, mul = 1, i = offset; i < offset + bitLength / 8; i++)
          result += array[i] * mul, mul *= 256;
        return result;
      }
      exports.readUintLE = readUintLE;
      function writeUintBE(bitLength, value, out, offset) {
        if (out === void 0 && (out = new Uint8Array(bitLength / 8)), offset === void 0 && (offset = 0), bitLength % 8 !== 0)
          throw new Error("writeUintBE supports only bitLengths divisible by 8");
        if (!int_1.isSafeInteger(value))
          throw new Error("writeUintBE value must be an integer");
        for (var div = 1, i = bitLength / 8 + offset - 1; i >= offset; i--)
          out[i] = value / div & 255, div *= 256;
        return out;
      }
      exports.writeUintBE = writeUintBE;
      function writeUintLE(bitLength, value, out, offset) {
        if (out === void 0 && (out = new Uint8Array(bitLength / 8)), offset === void 0 && (offset = 0), bitLength % 8 !== 0)
          throw new Error("writeUintLE supports only bitLengths divisible by 8");
        if (!int_1.isSafeInteger(value))
          throw new Error("writeUintLE value must be an integer");
        for (var div = 1, i = offset; i < offset + bitLength / 8; i++)
          out[i] = value / div & 255, div *= 256;
        return out;
      }
      exports.writeUintLE = writeUintLE;
      function readFloat32BE(array, offset) {
        offset === void 0 && (offset = 0);
        var view = new DataView(array.buffer, array.byteOffset, array.byteLength);
        return view.getFloat32(offset);
      }
      exports.readFloat32BE = readFloat32BE;
      function readFloat32LE(array, offset) {
        offset === void 0 && (offset = 0);
        var view = new DataView(array.buffer, array.byteOffset, array.byteLength);
        return view.getFloat32(offset, !0);
      }
      exports.readFloat32LE = readFloat32LE;
      function readFloat64BE(array, offset) {
        offset === void 0 && (offset = 0);
        var view = new DataView(array.buffer, array.byteOffset, array.byteLength);
        return view.getFloat64(offset);
      }
      exports.readFloat64BE = readFloat64BE;
      function readFloat64LE(array, offset) {
        offset === void 0 && (offset = 0);
        var view = new DataView(array.buffer, array.byteOffset, array.byteLength);
        return view.getFloat64(offset, !0);
      }
      exports.readFloat64LE = readFloat64LE;
      function writeFloat32BE(value, out, offset) {
        out === void 0 && (out = new Uint8Array(4)), offset === void 0 && (offset = 0);
        var view = new DataView(out.buffer, out.byteOffset, out.byteLength);
        return view.setFloat32(offset, value), out;
      }
      exports.writeFloat32BE = writeFloat32BE;
      function writeFloat32LE(value, out, offset) {
        out === void 0 && (out = new Uint8Array(4)), offset === void 0 && (offset = 0);
        var view = new DataView(out.buffer, out.byteOffset, out.byteLength);
        return view.setFloat32(offset, value, !0), out;
      }
      exports.writeFloat32LE = writeFloat32LE;
      function writeFloat64BE(value, out, offset) {
        out === void 0 && (out = new Uint8Array(8)), offset === void 0 && (offset = 0);
        var view = new DataView(out.buffer, out.byteOffset, out.byteLength);
        return view.setFloat64(offset, value), out;
      }
      exports.writeFloat64BE = writeFloat64BE;
      function writeFloat64LE(value, out, offset) {
        out === void 0 && (out = new Uint8Array(8)), offset === void 0 && (offset = 0);
        var view = new DataView(out.buffer, out.byteOffset, out.byteLength);
        return view.setFloat64(offset, value, !0), out;
      }
      exports.writeFloat64LE = writeFloat64LE;
    }
  });

  // ../../node_modules/.pnpm/@stablelib+wipe@1.0.1/node_modules/@stablelib/wipe/lib/wipe.js
  var require_wipe = __commonJS({
    "../../node_modules/.pnpm/@stablelib+wipe@1.0.1/node_modules/@stablelib/wipe/lib/wipe.js"(exports) {
      "use strict";
      Object.defineProperty(exports, "__esModule", { value: !0 });
      function wipe(array) {
        for (var i = 0; i < array.length; i++)
          array[i] = 0;
        return array;
      }
      exports.wipe = wipe;
    }
  });

  // ../../node_modules/.pnpm/@stablelib+sha256@1.0.1/node_modules/@stablelib/sha256/lib/sha256.js
  var require_sha256 = __commonJS({
    "../../node_modules/.pnpm/@stablelib+sha256@1.0.1/node_modules/@stablelib/sha256/lib/sha256.js"(exports) {
      "use strict";
      Object.defineProperty(exports, "__esModule", { value: !0 });
      var binary_1 = require_binary(), wipe_1 = require_wipe();
      exports.DIGEST_LENGTH = 32;
      exports.BLOCK_SIZE = 64;
      var SHA256 = (
        /** @class */
        (function() {
          function SHA2562() {
            this.digestLength = exports.DIGEST_LENGTH, this.blockSize = exports.BLOCK_SIZE, this._state = new Int32Array(8), this._temp = new Int32Array(64), this._buffer = new Uint8Array(128), this._bufferLength = 0, this._bytesHashed = 0, this._finished = !1, this.reset();
          }
          return SHA2562.prototype._initState = function() {
            this._state[0] = 1779033703, this._state[1] = 3144134277, this._state[2] = 1013904242, this._state[3] = 2773480762, this._state[4] = 1359893119, this._state[5] = 2600822924, this._state[6] = 528734635, this._state[7] = 1541459225;
          }, SHA2562.prototype.reset = function() {
            return this._initState(), this._bufferLength = 0, this._bytesHashed = 0, this._finished = !1, this;
          }, SHA2562.prototype.clean = function() {
            wipe_1.wipe(this._buffer), wipe_1.wipe(this._temp), this.reset();
          }, SHA2562.prototype.update = function(data, dataLength) {
            if (dataLength === void 0 && (dataLength = data.length), this._finished)
              throw new Error("SHA256: can't update because hash was finished.");
            var dataPos = 0;
            if (this._bytesHashed += dataLength, this._bufferLength > 0) {
              for (; this._bufferLength < this.blockSize && dataLength > 0; )
                this._buffer[this._bufferLength++] = data[dataPos++], dataLength--;
              this._bufferLength === this.blockSize && (hashBlocks(this._temp, this._state, this._buffer, 0, this.blockSize), this._bufferLength = 0);
            }
            for (dataLength >= this.blockSize && (dataPos = hashBlocks(this._temp, this._state, data, dataPos, dataLength), dataLength %= this.blockSize); dataLength > 0; )
              this._buffer[this._bufferLength++] = data[dataPos++], dataLength--;
            return this;
          }, SHA2562.prototype.finish = function(out) {
            if (!this._finished) {
              var bytesHashed = this._bytesHashed, left = this._bufferLength, bitLenHi = bytesHashed / 536870912 | 0, bitLenLo = bytesHashed << 3, padLength = bytesHashed % 64 < 56 ? 64 : 128;
              this._buffer[left] = 128;
              for (var i = left + 1; i < padLength - 8; i++)
                this._buffer[i] = 0;
              binary_1.writeUint32BE(bitLenHi, this._buffer, padLength - 8), binary_1.writeUint32BE(bitLenLo, this._buffer, padLength - 4), hashBlocks(this._temp, this._state, this._buffer, 0, padLength), this._finished = !0;
            }
            for (var i = 0; i < this.digestLength / 4; i++)
              binary_1.writeUint32BE(this._state[i], out, i * 4);
            return this;
          }, SHA2562.prototype.digest = function() {
            var out = new Uint8Array(this.digestLength);
            return this.finish(out), out;
          }, SHA2562.prototype.saveState = function() {
            if (this._finished)
              throw new Error("SHA256: cannot save finished state");
            return {
              state: new Int32Array(this._state),
              buffer: this._bufferLength > 0 ? new Uint8Array(this._buffer) : void 0,
              bufferLength: this._bufferLength,
              bytesHashed: this._bytesHashed
            };
          }, SHA2562.prototype.restoreState = function(savedState) {
            return this._state.set(savedState.state), this._bufferLength = savedState.bufferLength, savedState.buffer && this._buffer.set(savedState.buffer), this._bytesHashed = savedState.bytesHashed, this._finished = !1, this;
          }, SHA2562.prototype.cleanSavedState = function(savedState) {
            wipe_1.wipe(savedState.state), savedState.buffer && wipe_1.wipe(savedState.buffer), savedState.bufferLength = 0, savedState.bytesHashed = 0;
          }, SHA2562;
        })()
      );
      exports.SHA256 = SHA256;
      var K = new Int32Array([
        1116352408,
        1899447441,
        3049323471,
        3921009573,
        961987163,
        1508970993,
        2453635748,
        2870763221,
        3624381080,
        310598401,
        607225278,
        1426881987,
        1925078388,
        2162078206,
        2614888103,
        3248222580,
        3835390401,
        4022224774,
        264347078,
        604807628,
        770255983,
        1249150122,
        1555081692,
        1996064986,
        2554220882,
        2821834349,
        2952996808,
        3210313671,
        3336571891,
        3584528711,
        113926993,
        338241895,
        666307205,
        773529912,
        1294757372,
        1396182291,
        1695183700,
        1986661051,
        2177026350,
        2456956037,
        2730485921,
        2820302411,
        3259730800,
        3345764771,
        3516065817,
        3600352804,
        4094571909,
        275423344,
        430227734,
        506948616,
        659060556,
        883997877,
        958139571,
        1322822218,
        1537002063,
        1747873779,
        1955562222,
        2024104815,
        2227730452,
        2361852424,
        2428436474,
        2756734187,
        3204031479,
        3329325298
      ]);
      function hashBlocks(w, v, p, pos, len) {
        for (; len >= 64; ) {
          for (var a = v[0], b = v[1], c = v[2], d = v[3], e = v[4], f = v[5], g = v[6], h = v[7], i = 0; i < 16; i++) {
            var j = pos + i * 4;
            w[i] = binary_1.readUint32BE(p, j);
          }
          for (var i = 16; i < 64; i++) {
            var u = w[i - 2], t1 = (u >>> 17 | u << 15) ^ (u >>> 19 | u << 13) ^ u >>> 10;
            u = w[i - 15];
            var t2 = (u >>> 7 | u << 25) ^ (u >>> 18 | u << 14) ^ u >>> 3;
            w[i] = (t1 + w[i - 7] | 0) + (t2 + w[i - 16] | 0);
          }
          for (var i = 0; i < 64; i++) {
            var t1 = (((e >>> 6 | e << 26) ^ (e >>> 11 | e << 21) ^ (e >>> 25 | e << 7)) + (e & f ^ ~e & g) | 0) + (h + (K[i] + w[i] | 0) | 0) | 0, t2 = ((a >>> 2 | a << 30) ^ (a >>> 13 | a << 19) ^ (a >>> 22 | a << 10)) + (a & b ^ a & c ^ b & c) | 0;
            h = g, g = f, f = e, e = d + t1 | 0, d = c, c = b, b = a, a = t1 + t2 | 0;
          }
          v[0] += a, v[1] += b, v[2] += c, v[3] += d, v[4] += e, v[5] += f, v[6] += g, v[7] += h, pos += 64, len -= 64;
        }
        return pos;
      }
      function hash2(data) {
        var h = new SHA256();
        h.update(data);
        var digest = h.digest();
        return h.clean(), digest;
      }
      exports.hash = hash2;
    }
  });

  // src/commerce.ts
  var commerce_exports = {};
  __export(commerce_exports, {
    createMiniGameRuntime: () => createMiniGameRuntime,
    createXiaohongshuPlatform: () => createXiaohongshuPlatform
  });

  // src/index.ts
  var import_sha256 = __toESM(require_sha256()), WINDOW_MS = 3e4, UUID_PATTERN = /^(?:[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}|00000000-0000-0000-0000-000000000000|ffffffff-ffff-ffff-ffff-ffffffffffff)$/, INSTALLATION_DOMAIN = asciiBytes("ViceMe.MiniGame.Installation.v1\0"), CODE_DOMAIN = asciiBytes("ViceMe.MiniGame.UnlockCode.v2\0");
  function installationDigest(installationId) {
    let message = new Uint8Array(INSTALLATION_DOMAIN.length + 16);
    return message.set(INSTALLATION_DOMAIN), message.set(uuidBytes(installationId), INSTALLATION_DOMAIN.length), (0, import_sha256.hash)(message).slice(0, 16);
  }
  function verifyUnlockCode(code, configuration, nowMs = Date.now()) {
    try {
      if (typeof code != "string" || code.length !== 6 || !/^[0-9]{6}$/.test(code))
        return !1;
      let window = windowNumber(nowMs), prepared = prepareCode({
        workId: configuration.workId,
        itemId: configuration.itemId,
        environment: configuration.environment,
        sharedSecret: configuration.sharedSecret,
        installationDigest: installationDigest(configuration.installationId)
      });
      return codeAt(prepared, window) === code || window > 0 && codeAt(prepared, window - 1) === code || codeAt(prepared, window + 1) === code;
    } catch (e) {
      return !1;
    }
  }
  function windowNumber(nowMs) {
    let window = Math.floor(nowMs / WINDOW_MS);
    if (!Number.isSafeInteger(nowMs) || nowMs < 0 || (window + 1) * WINDOW_MS > 864e13)
      throw new TypeError("Invalid unlock code time");
    return window;
  }
  function prepareCode(input) {
    if (typeof input.sharedSecret != "string" || input.sharedSecret.length !== 64 || !/^[0-9a-f]{64}$/.test(input.sharedSecret))
      throw new TypeError("Invalid unlock code shared secret");
    if (!(input.installationDigest instanceof Uint8Array) || input.installationDigest.length !== 16)
      throw new TypeError("Invalid installation digest");
    let key = hexBytes(input.sharedSecret), inner = new Uint8Array(64 + CODE_DOMAIN.length + 16 + 1 + 16 + 16 + 8), outer = new Uint8Array(96);
    inner.fill(54, 0, 64), outer.fill(92, 0, 64);
    for (let index = 0; index < key.length; index++)
      inner[index] = inner[index] ^ key[index], outer[index] = outer[index] ^ key[index];
    let offset = 64;
    return inner.set(CODE_DOMAIN, offset), offset += CODE_DOMAIN.length, inner.set(uuidBytes(input.workId), offset), offset += 16, inner[offset++] = environmentByte(input.environment), inner.set(input.installationDigest, offset), offset += 16, inner.set(uuidBytes(input.itemId), offset), offset += 16, { inner, outer, counter: new DataView(inner.buffer, offset, 8) };
  }
  function codeAt(prepared, window) {
    prepared.counter.setUint32(0, Math.floor(window / 4294967296)), prepared.counter.setUint32(4, window % 4294967296), prepared.outer.set((0, import_sha256.hash)(prepared.inner), 64);
    let digest = (0, import_sha256.hash)(prepared.outer), offset = digest[31] & 15;
    return (((digest[offset] & 127) << 24 | digest[offset + 1] << 16 | digest[offset + 2] << 8 | digest[offset + 3]) % 1e6).toString().padStart(6, "0");
  }
  function environmentByte(environment) {
    if (environment === "SANDBOX") return 16;
    if (environment === "PRODUCTION") return 17;
    throw new TypeError("Invalid unlock code environment");
  }
  function uuidBytes(value) {
    if (typeof value != "string" || value.length !== 36 || !UUID_PATTERN.test(value))
      throw new TypeError("Invalid mini-game UUID");
    return hexBytes(value.replace(/-/g, ""));
  }
  function asciiBytes(value) {
    let bytes = new Uint8Array(value.length);
    for (let index = 0; index < value.length; index++)
      bytes[index] = value.charCodeAt(index);
    return bytes;
  }
  function hexBytes(value) {
    let bytes = new Uint8Array(value.length / 2);
    for (let index = 0; index < bytes.length; index++)
      bytes[index] = parseInt(value.slice(index * 2, index * 2 + 2), 16);
    return bytes;
  }

  // src/runtime.ts
  var UUID_PATTERN2 = /^(?:[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}|00000000-0000-0000-0000-000000000000|ffffffff-ffff-ffff-ffff-ffffffffffff)$/, MESSAGES = {
    CONFIGURATION_INVALID: "\u8D2D\u4E70\u914D\u7F6E\u65E0\u6548\uFF0C\u8BF7\u8054\u7CFB\u6E38\u620F\u4F5C\u8005\u66F4\u65B0\u540E\u91CD\u8BD5\u3002",
    UNSUPPORTED_CLIENT: "\u8D2D\u4E70\u529F\u80FD\u9700\u8981\u5C0F\u7EA2\u4E66 9.46.0 \u6216\u66F4\u65B0\u7248\u672C\uFF0C\u8BF7\u5347\u7EA7\u540E\u91CD\u8BD5\uFF1B\u666E\u901A\u6E38\u620F\u4E0D\u53D7\u5F71\u54CD\u3002",
    STORAGE_UNAVAILABLE: "\u65E0\u6CD5\u4F7F\u7528\u5B89\u5168\u5B58\u50A8\uFF0C\u8BF7\u91CD\u65B0\u6253\u5F00\u6E38\u620F\u540E\u91CD\u8BD5\uFF1B\u666E\u901A\u6E38\u620F\u4E0D\u53D7\u5F71\u54CD\u3002",
    STORAGE_READ_FAILED: "\u65E0\u6CD5\u8BFB\u53D6\u8D2D\u4E70\u8BB0\u5F55\uFF0C\u8BF7\u91CD\u65B0\u6253\u5F00\u6E38\u620F\u540E\u91CD\u8BD5\uFF0C\u4E0D\u8981\u6E05\u9664\u6E38\u620F\u6570\u636E\u3002",
    STORAGE_WRITE_FAILED: "\u8D2D\u4E70\u8BB0\u5F55\u672A\u80FD\u4FDD\u5B58\uFF0C\u8BF7\u68C0\u67E5\u5B58\u50A8\u7A7A\u95F4\u540E\u91CD\u8BD5\u3002",
    STORAGE_READBACK_FAILED: "\u8D2D\u4E70\u8BB0\u5F55\u4FDD\u5B58\u540E\u6821\u9A8C\u5931\u8D25\uFF0C\u8BF7\u91CD\u65B0\u6253\u5F00\u6E38\u620F\u540E\u91CD\u8BD5\uFF0C\u4E0D\u8981\u6E05\u9664\u6E38\u620F\u6570\u636E\u3002",
    INSTALLATION_CORRUPT: "\u8D2D\u4E70\u8BB0\u5F55\u635F\u574F\u6216\u7248\u672C\u4E0D\u517C\u5BB9\uFF0C\u8BF7\u8054\u7CFB\u6E38\u620F\u4F5C\u8005\u5347\u7EA7\u5E76\u91CD\u65B0\u63A5\u5165\uFF0C\u4E0D\u8981\u6E05\u9664\u6E38\u620F\u6570\u636E\u3002",
    NOT_INITIALIZED: "\u8D2D\u4E70\u529F\u80FD\u5C1A\u672A\u5C31\u7EEA\uFF0C\u8BF7\u5148\u91CD\u65B0\u521D\u59CB\u5316\uFF1B\u666E\u901A\u6E38\u620F\u4E0D\u53D7\u5F71\u54CD\u3002",
    ITEM_NOT_FOUND: "\u6E38\u620F\u672A\u914D\u7F6E\u6B64\u9053\u5177\uFF0C\u8BF7\u8054\u7CFB\u6E38\u620F\u4F5C\u8005\u66F4\u65B0\u3002",
    RANDOM_UNAVAILABLE: "\u65E0\u6CD5\u521B\u5EFA\u5B89\u5168\u7684\u8D2D\u4E70\u5B9E\u4F8B\uFF0C\u8BF7\u91CD\u65B0\u6253\u5F00\u6E38\u620F\u540E\u91CD\u8BD5\u3002",
    CARD_SAVE_FAILED: "\u4ED8\u6B3E\u5361\u672A\u80FD\u4FDD\u5B58\uFF0C\u8BF7\u5141\u8BB8\u4FDD\u5B58\u5230\u76F8\u518C\u540E\u91CD\u8BD5\u3002",
    MINI_GAME_UNLOCK_CODE_INVALID: "\u516D\u4F4D\u52A8\u6001\u7801\u65E0\u6548\u3001\u5DF2\u8FC7\u671F\u6216\u4E0D\u5C5E\u4E8E\u5F53\u524D\u6E38\u620F\u5B89\u88C5\u3002\u8BF7\u68C0\u67E5\u8BBE\u5907\u65F6\u95F4\uFF0C\u5E76\u4ECE\u672C\u5B89\u88C5\u4ED8\u6B3E\u5361\u83B7\u53D6\u65B0\u7801\u3002",
    RUNTIME_UNAVAILABLE: "\u8D2D\u4E70\u529F\u80FD\u6682\u4E0D\u53EF\u7528\uFF0C\u8BF7\u91CD\u65B0\u6253\u5F00\u6E38\u620F\u540E\u91CD\u8BD5\uFF1B\u666E\u901A\u6E38\u620F\u4E0D\u53D7\u5F71\u54CD\u3002"
  };
  function failure(code) {
    return {
      ok: !1,
      code,
      message: MESSAGES[code] || MESSAGES.RUNTIME_UNAVAILABLE
    };
  }
  function isUuid(value) {
    return typeof value == "string" && value.length === 36 && UUID_PATTERN2.test(value);
  }
  function isObject(value) {
    return value !== null && typeof value == "object" && !Array.isArray(value);
  }
  function exactKeys(value, keys) {
    return Object.keys(value).length === keys.length && keys.every((key) => Object.prototype.hasOwnProperty.call(value, key));
  }
  function nonempty(value) {
    return typeof value == "string" && value.trim().length > 0;
  }
  function validConfiguration(value) {
    if (!isObject(value) || !exactKeys(value, [
      "protocolVersion",
      "workId",
      "workTitle",
      "environment",
      "publicClientId",
      "sharedSecret",
      "checkoutOrigin",
      "items"
    ]) || value.protocolVersion !== 2 || !isUuid(value.workId) || !nonempty(value.workTitle) || value.environment !== "SANDBOX" && value.environment !== "PRODUCTION" || typeof value.publicClientId != "string" || value.publicClientId.length !== 36 || !/^vca_[A-Za-z0-9_-]{32}$/.test(value.publicClientId) || typeof value.sharedSecret != "string" || value.sharedSecret.length !== 64 || !/^[0-9a-f]{64}$/.test(value.sharedSecret) || typeof value.checkoutOrigin != "string" || !Array.isArray(value.items))
      return !1;
    try {
      let origin = new URL(value.checkoutOrigin);
      if (origin.protocol !== "https:" || origin.origin !== value.checkoutOrigin)
        return !1;
    } catch (e) {
      return !1;
    }
    let aliases = /* @__PURE__ */ new Set(), ids = /* @__PURE__ */ new Set();
    for (let item of value.items) {
      if (!isObject(item) || !exactKeys(item, ["id", "alias", "title"]) || !isUuid(item.id) || !nonempty(item.alias) || item.alias !== item.alias.trim() || !nonempty(item.title) || aliases.has(item.alias) || ids.has(item.id))
        return !1;
      aliases.add(item.alias), ids.add(item.id);
    }
    return !0;
  }
  function createMiniGameRuntime(configuration, platform) {
    let config = validConfiguration(configuration) ? __spreadProps(__spreadValues({}, configuration), {
      items: configuration.items.map((item) => __spreadValues({}, item))
    }) : null, storageKey = config ? "viceme:mini-game:v1:".concat(config.workId, ":").concat(config.environment, ":").concat(config.publicClientId) : "", items = new Map(
      config ? config.items.map((item) => [item.alias, item]) : []
    ), launches = /* @__PURE__ */ new Map(), record = null, pendingInstallationId = null, observedInstallation = !1, queue = Promise.resolve();
    function serialize(operation) {
      let result = queue.then(async () => {
        if (!config) return failure("CONFIGURATION_INVALID");
        try {
          return await operation();
        } catch (error) {
          return failure(
            isObject(error) && typeof error.code == "string" && Object.prototype.hasOwnProperty.call(MESSAGES, error.code) ? error.code : "RUNTIME_UNAVAILABLE"
          );
        }
      });
      return queue = result.then(
        () => {
        },
        () => {
        }
      ), result;
    }
    async function storage(operation, code) {
      try {
        return await operation();
      } catch (error) {
        throw isObject(error) && typeof error.code == "string" && Object.prototype.hasOwnProperty.call(MESSAGES, error.code) ? error : Object.assign(new Error(code), { code });
      }
    }
    function parseRecord(value) {
      if (!isObject(value) || !exactKeys(value, [
        "version",
        "workId",
        "environment",
        "publicClientId",
        "installationId",
        "unlockedItems"
      ]) || value.version !== 2 || value.workId !== config.workId || value.environment !== config.environment || value.publicClientId !== config.publicClientId || !isUuid(value.installationId) || !isObject(value.unlockedItems))
        return null;
      let unlockedItems = {};
      for (let itemId of Object.keys(value.unlockedItems)) {
        if (!isUuid(itemId) || value.unlockedItems[itemId] !== !0) return null;
        unlockedItems[itemId] = !0;
      }
      return {
        version: 2,
        workId: config.workId,
        environment: config.environment,
        publicClientId: config.publicClientId,
        installationId: value.installationId,
        unlockedItems
      };
    }
    async function persist(next) {
      await storage(
        () => platform.write(storageKey, next),
        "STORAGE_WRITE_FAILED"
      );
      let restored = parseRecord(
        await storage(() => platform.read(storageKey), "STORAGE_READ_FAILED")
      );
      return !restored || restored.installationId !== next.installationId || Object.keys(restored.unlockedItems).length !== Object.keys(next.unlockedItems).length || Object.keys(next.unlockedItems).some(
        (id) => restored.unlockedItems[id] !== next.unlockedItems[id]
      ) ? !1 : (record = restored, !0);
    }
    async function redeem(alias, code) {
      if (!record) return failure("NOT_INITIALIZED");
      let item = items.get(alias);
      if (!item) return failure("ITEM_NOT_FOUND");
      if (!verifyUnlockCode(code, {
        workId: config.workId,
        environment: config.environment,
        sharedSecret: config.sharedSecret,
        itemId: item.id,
        installationId: record.installationId
      })) return failure("MINI_GAME_UNLOCK_CODE_INVALID");
      if (!record.unlockedItems[item.id]) {
        let next = __spreadProps(__spreadValues({}, record), {
          unlockedItems: __spreadProps(__spreadValues({}, record.unlockedItems), { [item.id]: !0 })
        });
        if (!await persist(next)) return failure("STORAGE_READBACK_FAILED");
      }
      return { ok: !0, unlocked: !0 };
    }
    return {
      initialize() {
        return serialize(async () => {
          record = null, await platform.checkSupport();
          let stored = await storage(
            () => platform.read(storageKey),
            "STORAGE_READ_FAILED"
          ), next;
          if (stored === null) {
            if (observedInstallation) return failure("INSTALLATION_CORRUPT");
            if (pendingInstallationId || (pendingInstallationId = platform.randomId()), !isUuid(pendingInstallationId))
              return failure("RANDOM_UNAVAILABLE");
            next = {
              version: 2,
              workId: config.workId,
              environment: config.environment,
              publicClientId: config.publicClientId,
              installationId: pendingInstallationId,
              unlockedItems: {}
            };
          } else {
            observedInstallation = !0;
            let restored = parseRecord(stored);
            if (!restored || pendingInstallationId && pendingInstallationId !== restored.installationId)
              return failure("INSTALLATION_CORRUPT");
            next = restored;
          }
          return await persist(next) ? (observedInstallation = !0, pendingInstallationId = next.installationId, { ok: !0 }) : failure("STORAGE_READBACK_FAILED");
        });
      },
      createPurchaseCard(alias) {
        return serialize(async () => {
          if (!record) return failure("NOT_INITIALIZED");
          let item = items.get(alias);
          if (!item) return failure("ITEM_NOT_FOUND");
          let launchId = launches.get(item.id);
          if (!launchId) {
            if (launchId = platform.randomId(), !isUuid(launchId)) return failure("RANDOM_UNAVAILABLE");
            launches.set(item.id, launchId);
          }
          let checkoutUrl = "".concat(config.checkoutOrigin, "/checkout/mini-game?v=1") + "&publicClientId=".concat(config.publicClientId, "&itemId=").concat(item.id) + "&installationId=".concat(record.installationId, "&runtimeVersion=2.0.0&launchId=").concat(launchId);
          return await storage(
            () => platform.saveCard({
              checkoutUrl,
              workTitle: config.workTitle,
              itemTitle: item.title,
              environment: config.environment
            }),
            "CARD_SAVE_FAILED"
          ), { ok: !0, checkoutUrl };
        });
      },
      redeemCode(alias, code) {
        return serialize(() => redeem(alias, code));
      },
      isUnlocked(alias) {
        return serialize(async () => {
          if (!record) return failure("NOT_INITIALIZED");
          let item = items.get(alias);
          return item ? {
            ok: !0,
            unlocked: Object.prototype.hasOwnProperty.call(
              record.unlockedItems,
              item.id
            )
          } : failure("ITEM_NOT_FOUND");
        });
      }
    };
  }

  // ../../node_modules/.pnpm/qrcode-generator@2.0.4/node_modules/qrcode-generator/dist/qrcode.mjs
  var qrcode = function(typeNumber, errorCorrectionLevel) {
    let _typeNumber = typeNumber, _errorCorrectionLevel = QRErrorCorrectionLevel[errorCorrectionLevel], _modules = null, _moduleCount = 0, _dataCache = null, _dataList = [], _this = {}, makeImpl = function(test, maskPattern) {
      _moduleCount = _typeNumber * 4 + 17, _modules = (function(moduleCount) {
        let modules = new Array(moduleCount);
        for (let row = 0; row < moduleCount; row += 1) {
          modules[row] = new Array(moduleCount);
          for (let col = 0; col < moduleCount; col += 1)
            modules[row][col] = null;
        }
        return modules;
      })(_moduleCount), setupPositionProbePattern(0, 0), setupPositionProbePattern(_moduleCount - 7, 0), setupPositionProbePattern(0, _moduleCount - 7), setupPositionAdjustPattern(), setupTimingPattern(), setupTypeInfo(test, maskPattern), _typeNumber >= 7 && setupTypeNumber(test), _dataCache == null && (_dataCache = createData(_typeNumber, _errorCorrectionLevel, _dataList)), mapData(_dataCache, maskPattern);
    }, setupPositionProbePattern = function(row, col) {
      for (let r = -1; r <= 7; r += 1)
        if (!(row + r <= -1 || _moduleCount <= row + r))
          for (let c = -1; c <= 7; c += 1)
            col + c <= -1 || _moduleCount <= col + c || (0 <= r && r <= 6 && (c == 0 || c == 6) || 0 <= c && c <= 6 && (r == 0 || r == 6) || 2 <= r && r <= 4 && 2 <= c && c <= 4 ? _modules[row + r][col + c] = !0 : _modules[row + r][col + c] = !1);
    }, getBestMaskPattern = function() {
      let minLostPoint = 0, pattern = 0;
      for (let i = 0; i < 8; i += 1) {
        makeImpl(!0, i);
        let lostPoint = QRUtil.getLostPoint(_this);
        (i == 0 || minLostPoint > lostPoint) && (minLostPoint = lostPoint, pattern = i);
      }
      return pattern;
    }, setupTimingPattern = function() {
      for (let r = 8; r < _moduleCount - 8; r += 1)
        _modules[r][6] == null && (_modules[r][6] = r % 2 == 0);
      for (let c = 8; c < _moduleCount - 8; c += 1)
        _modules[6][c] == null && (_modules[6][c] = c % 2 == 0);
    }, setupPositionAdjustPattern = function() {
      let pos = QRUtil.getPatternPosition(_typeNumber);
      for (let i = 0; i < pos.length; i += 1)
        for (let j = 0; j < pos.length; j += 1) {
          let row = pos[i], col = pos[j];
          if (_modules[row][col] == null)
            for (let r = -2; r <= 2; r += 1)
              for (let c = -2; c <= 2; c += 1)
                r == -2 || r == 2 || c == -2 || c == 2 || r == 0 && c == 0 ? _modules[row + r][col + c] = !0 : _modules[row + r][col + c] = !1;
        }
    }, setupTypeNumber = function(test) {
      let bits = QRUtil.getBCHTypeNumber(_typeNumber);
      for (let i = 0; i < 18; i += 1) {
        let mod = !test && (bits >> i & 1) == 1;
        _modules[Math.floor(i / 3)][i % 3 + _moduleCount - 8 - 3] = mod;
      }
      for (let i = 0; i < 18; i += 1) {
        let mod = !test && (bits >> i & 1) == 1;
        _modules[i % 3 + _moduleCount - 8 - 3][Math.floor(i / 3)] = mod;
      }
    }, setupTypeInfo = function(test, maskPattern) {
      let data = _errorCorrectionLevel << 3 | maskPattern, bits = QRUtil.getBCHTypeInfo(data);
      for (let i = 0; i < 15; i += 1) {
        let mod = !test && (bits >> i & 1) == 1;
        i < 6 ? _modules[i][8] = mod : i < 8 ? _modules[i + 1][8] = mod : _modules[_moduleCount - 15 + i][8] = mod;
      }
      for (let i = 0; i < 15; i += 1) {
        let mod = !test && (bits >> i & 1) == 1;
        i < 8 ? _modules[8][_moduleCount - i - 1] = mod : i < 9 ? _modules[8][15 - i - 1 + 1] = mod : _modules[8][15 - i - 1] = mod;
      }
      _modules[_moduleCount - 8][8] = !test;
    }, mapData = function(data, maskPattern) {
      let inc = -1, row = _moduleCount - 1, bitIndex = 7, byteIndex = 0, maskFunc = QRUtil.getMaskFunction(maskPattern);
      for (let col = _moduleCount - 1; col > 0; col -= 2)
        for (col == 6 && (col -= 1); ; ) {
          for (let c = 0; c < 2; c += 1)
            if (_modules[row][col - c] == null) {
              let dark = !1;
              byteIndex < data.length && (dark = (data[byteIndex] >>> bitIndex & 1) == 1), maskFunc(row, col - c) && (dark = !dark), _modules[row][col - c] = dark, bitIndex -= 1, bitIndex == -1 && (byteIndex += 1, bitIndex = 7);
            }
          if (row += inc, row < 0 || _moduleCount <= row) {
            row -= inc, inc = -inc;
            break;
          }
        }
    }, createBytes = function(buffer, rsBlocks) {
      let offset = 0, maxDcCount = 0, maxEcCount = 0, dcdata = new Array(rsBlocks.length), ecdata = new Array(rsBlocks.length);
      for (let r = 0; r < rsBlocks.length; r += 1) {
        let dcCount = rsBlocks[r].dataCount, ecCount = rsBlocks[r].totalCount - dcCount;
        maxDcCount = Math.max(maxDcCount, dcCount), maxEcCount = Math.max(maxEcCount, ecCount), dcdata[r] = new Array(dcCount);
        for (let i = 0; i < dcdata[r].length; i += 1)
          dcdata[r][i] = 255 & buffer.getBuffer()[i + offset];
        offset += dcCount;
        let rsPoly = QRUtil.getErrorCorrectPolynomial(ecCount), modPoly = qrPolynomial(dcdata[r], rsPoly.getLength() - 1).mod(rsPoly);
        ecdata[r] = new Array(rsPoly.getLength() - 1);
        for (let i = 0; i < ecdata[r].length; i += 1) {
          let modIndex = i + modPoly.getLength() - ecdata[r].length;
          ecdata[r][i] = modIndex >= 0 ? modPoly.getAt(modIndex) : 0;
        }
      }
      let totalCodeCount = 0;
      for (let i = 0; i < rsBlocks.length; i += 1)
        totalCodeCount += rsBlocks[i].totalCount;
      let data = new Array(totalCodeCount), index = 0;
      for (let i = 0; i < maxDcCount; i += 1)
        for (let r = 0; r < rsBlocks.length; r += 1)
          i < dcdata[r].length && (data[index] = dcdata[r][i], index += 1);
      for (let i = 0; i < maxEcCount; i += 1)
        for (let r = 0; r < rsBlocks.length; r += 1)
          i < ecdata[r].length && (data[index] = ecdata[r][i], index += 1);
      return data;
    }, createData = function(typeNumber2, errorCorrectionLevel2, dataList) {
      let rsBlocks = QRRSBlock.getRSBlocks(typeNumber2, errorCorrectionLevel2), buffer = qrBitBuffer();
      for (let i = 0; i < dataList.length; i += 1) {
        let data = dataList[i];
        buffer.put(data.getMode(), 4), buffer.put(data.getLength(), QRUtil.getLengthInBits(data.getMode(), typeNumber2)), data.write(buffer);
      }
      let totalDataCount = 0;
      for (let i = 0; i < rsBlocks.length; i += 1)
        totalDataCount += rsBlocks[i].dataCount;
      if (buffer.getLengthInBits() > totalDataCount * 8)
        throw "code length overflow. (" + buffer.getLengthInBits() + ">" + totalDataCount * 8 + ")";
      for (buffer.getLengthInBits() + 4 <= totalDataCount * 8 && buffer.put(0, 4); buffer.getLengthInBits() % 8 != 0; )
        buffer.putBit(!1);
      for (; !(buffer.getLengthInBits() >= totalDataCount * 8 || (buffer.put(236, 8), buffer.getLengthInBits() >= totalDataCount * 8)); )
        buffer.put(17, 8);
      return createBytes(buffer, rsBlocks);
    };
    _this.addData = function(data, mode) {
      mode = mode || "Byte";
      let newData = null;
      switch (mode) {
        case "Numeric":
          newData = qrNumber(data);
          break;
        case "Alphanumeric":
          newData = qrAlphaNum(data);
          break;
        case "Byte":
          newData = qr8BitByte(data);
          break;
        case "Kanji":
          newData = qrKanji(data);
          break;
        default:
          throw "mode:" + mode;
      }
      _dataList.push(newData), _dataCache = null;
    }, _this.isDark = function(row, col) {
      if (row < 0 || _moduleCount <= row || col < 0 || _moduleCount <= col)
        throw row + "," + col;
      return _modules[row][col];
    }, _this.getModuleCount = function() {
      return _moduleCount;
    }, _this.make = function() {
      if (_typeNumber < 1) {
        let typeNumber2 = 1;
        for (; typeNumber2 < 40; typeNumber2++) {
          let rsBlocks = QRRSBlock.getRSBlocks(typeNumber2, _errorCorrectionLevel), buffer = qrBitBuffer();
          for (let i = 0; i < _dataList.length; i++) {
            let data = _dataList[i];
            buffer.put(data.getMode(), 4), buffer.put(data.getLength(), QRUtil.getLengthInBits(data.getMode(), typeNumber2)), data.write(buffer);
          }
          let totalDataCount = 0;
          for (let i = 0; i < rsBlocks.length; i++)
            totalDataCount += rsBlocks[i].dataCount;
          if (buffer.getLengthInBits() <= totalDataCount * 8)
            break;
        }
        _typeNumber = typeNumber2;
      }
      makeImpl(!1, getBestMaskPattern());
    }, _this.createTableTag = function(cellSize, margin) {
      cellSize = cellSize || 2, margin = typeof margin == "undefined" ? cellSize * 4 : margin;
      let qrHtml = "";
      qrHtml += '<table style="', qrHtml += " border-width: 0px; border-style: none;", qrHtml += " border-collapse: collapse;", qrHtml += " padding: 0px; margin: " + margin + "px;", qrHtml += '">', qrHtml += "<tbody>";
      for (let r = 0; r < _this.getModuleCount(); r += 1) {
        qrHtml += "<tr>";
        for (let c = 0; c < _this.getModuleCount(); c += 1)
          qrHtml += '<td style="', qrHtml += " border-width: 0px; border-style: none;", qrHtml += " border-collapse: collapse;", qrHtml += " padding: 0px; margin: 0px;", qrHtml += " width: " + cellSize + "px;", qrHtml += " height: " + cellSize + "px;", qrHtml += " background-color: ", qrHtml += _this.isDark(r, c) ? "#000000" : "#ffffff", qrHtml += ";", qrHtml += '"/>';
        qrHtml += "</tr>";
      }
      return qrHtml += "</tbody>", qrHtml += "</table>", qrHtml;
    }, _this.createSvgTag = function(cellSize, margin, alt, title) {
      let opts = {};
      typeof arguments[0] == "object" && (opts = arguments[0], cellSize = opts.cellSize, margin = opts.margin, alt = opts.alt, title = opts.title), cellSize = cellSize || 2, margin = typeof margin == "undefined" ? cellSize * 4 : margin, alt = typeof alt == "string" ? { text: alt } : alt || {}, alt.text = alt.text || null, alt.id = alt.text ? alt.id || "qrcode-description" : null, title = typeof title == "string" ? { text: title } : title || {}, title.text = title.text || null, title.id = title.text ? title.id || "qrcode-title" : null;
      let size = _this.getModuleCount() * cellSize + margin * 2, c, mc, r, mr, qrSvg = "", rect;
      for (rect = "l" + cellSize + ",0 0," + cellSize + " -" + cellSize + ",0 0,-" + cellSize + "z ", qrSvg += '<svg version="1.1" xmlns="http://www.w3.org/2000/svg"', qrSvg += opts.scalable ? "" : ' width="' + size + 'px" height="' + size + 'px"', qrSvg += ' viewBox="0 0 ' + size + " " + size + '" ', qrSvg += ' preserveAspectRatio="xMinYMin meet"', qrSvg += title.text || alt.text ? ' role="img" aria-labelledby="' + escapeXml([title.id, alt.id].join(" ").trim()) + '"' : "", qrSvg += ">", qrSvg += title.text ? '<title id="' + escapeXml(title.id) + '">' + escapeXml(title.text) + "</title>" : "", qrSvg += alt.text ? '<description id="' + escapeXml(alt.id) + '">' + escapeXml(alt.text) + "</description>" : "", qrSvg += '<rect width="100%" height="100%" fill="white" cx="0" cy="0"/>', qrSvg += '<path d="', r = 0; r < _this.getModuleCount(); r += 1)
        for (mr = r * cellSize + margin, c = 0; c < _this.getModuleCount(); c += 1)
          _this.isDark(r, c) && (mc = c * cellSize + margin, qrSvg += "M" + mc + "," + mr + rect);
      return qrSvg += '" stroke="transparent" fill="black"/>', qrSvg += "</svg>", qrSvg;
    }, _this.createDataURL = function(cellSize, margin) {
      cellSize = cellSize || 2, margin = typeof margin == "undefined" ? cellSize * 4 : margin;
      let size = _this.getModuleCount() * cellSize + margin * 2, min = margin, max = size - margin;
      return createDataURL(size, size, function(x, y) {
        if (min <= x && x < max && min <= y && y < max) {
          let c = Math.floor((x - min) / cellSize), r = Math.floor((y - min) / cellSize);
          return _this.isDark(r, c) ? 0 : 1;
        } else
          return 1;
      });
    }, _this.createImgTag = function(cellSize, margin, alt) {
      cellSize = cellSize || 2, margin = typeof margin == "undefined" ? cellSize * 4 : margin;
      let size = _this.getModuleCount() * cellSize + margin * 2, img = "";
      return img += "<img", img += ' src="', img += _this.createDataURL(cellSize, margin), img += '"', img += ' width="', img += size, img += '"', img += ' height="', img += size, img += '"', alt && (img += ' alt="', img += escapeXml(alt), img += '"'), img += "/>", img;
    };
    let escapeXml = function(s) {
      let escaped = "";
      for (let i = 0; i < s.length; i += 1) {
        let c = s.charAt(i);
        switch (c) {
          case "<":
            escaped += "&lt;";
            break;
          case ">":
            escaped += "&gt;";
            break;
          case "&":
            escaped += "&amp;";
            break;
          case '"':
            escaped += "&quot;";
            break;
          default:
            escaped += c;
            break;
        }
      }
      return escaped;
    }, _createHalfASCII = function(margin) {
      margin = typeof margin == "undefined" ? 2 : margin;
      let size = _this.getModuleCount() * 1 + margin * 2, min = margin, max = size - margin, y, x, r1, r2, p, blocks = {
        "\u2588\u2588": "\u2588",
        "\u2588 ": "\u2580",
        " \u2588": "\u2584",
        "  ": " "
      }, blocksLastLineNoMargin = {
        "\u2588\u2588": "\u2580",
        "\u2588 ": "\u2580",
        " \u2588": " ",
        "  ": " "
      }, ascii = "";
      for (y = 0; y < size; y += 2) {
        for (r1 = Math.floor((y - min) / 1), r2 = Math.floor((y + 1 - min) / 1), x = 0; x < size; x += 1)
          p = "\u2588", min <= x && x < max && min <= y && y < max && _this.isDark(r1, Math.floor((x - min) / 1)) && (p = " "), min <= x && x < max && min <= y + 1 && y + 1 < max && _this.isDark(r2, Math.floor((x - min) / 1)) ? p += " " : p += "\u2588", ascii += margin < 1 && y + 1 >= max ? blocksLastLineNoMargin[p] : blocks[p];
        ascii += "\n";
      }
      return size % 2 && margin > 0 ? ascii.substring(0, ascii.length - size - 1) + Array(size + 1).join("\u2580") : ascii.substring(0, ascii.length - 1);
    };
    return _this.createASCII = function(cellSize, margin) {
      if (cellSize = cellSize || 1, cellSize < 2)
        return _createHalfASCII(margin);
      cellSize -= 1, margin = typeof margin == "undefined" ? cellSize * 2 : margin;
      let size = _this.getModuleCount() * cellSize + margin * 2, min = margin, max = size - margin, y, x, r, p, white = Array(cellSize + 1).join("\u2588\u2588"), black = Array(cellSize + 1).join("  "), ascii = "", line = "";
      for (y = 0; y < size; y += 1) {
        for (r = Math.floor((y - min) / cellSize), line = "", x = 0; x < size; x += 1)
          p = 1, min <= x && x < max && min <= y && y < max && _this.isDark(r, Math.floor((x - min) / cellSize)) && (p = 0), line += p ? white : black;
        for (r = 0; r < cellSize; r += 1)
          ascii += line + "\n";
      }
      return ascii.substring(0, ascii.length - 1);
    }, _this.renderTo2dContext = function(context, cellSize) {
      cellSize = cellSize || 2;
      let length = _this.getModuleCount();
      for (let row = 0; row < length; row++)
        for (let col = 0; col < length; col++)
          context.fillStyle = _this.isDark(row, col) ? "black" : "white", context.fillRect(col * cellSize, row * cellSize, cellSize, cellSize);
    }, _this;
  };
  qrcode.stringToBytes = function(s) {
    let bytes = [];
    for (let i = 0; i < s.length; i += 1) {
      let c = s.charCodeAt(i);
      bytes.push(c & 255);
    }
    return bytes;
  };
  qrcode.createStringToBytes = function(unicodeData, numChars) {
    let unicodeMap = (function() {
      let bin = base64DecodeInputStream(unicodeData), read = function() {
        let b = bin.read();
        if (b == -1) throw "eof";
        return b;
      }, count = 0, unicodeMap2 = {};
      for (; ; ) {
        let b0 = bin.read();
        if (b0 == -1) break;
        let b1 = read(), b2 = read(), b3 = read(), k = String.fromCharCode(b0 << 8 | b1), v = b2 << 8 | b3;
        unicodeMap2[k] = v, count += 1;
      }
      if (count != numChars)
        throw count + " != " + numChars;
      return unicodeMap2;
    })(), unknownChar = 63;
    return function(s) {
      let bytes = [];
      for (let i = 0; i < s.length; i += 1) {
        let c = s.charCodeAt(i);
        if (c < 128)
          bytes.push(c);
        else {
          let b = unicodeMap[s.charAt(i)];
          typeof b == "number" ? (b & 255) == b ? bytes.push(b) : (bytes.push(b >>> 8), bytes.push(b & 255)) : bytes.push(unknownChar);
        }
      }
      return bytes;
    };
  };
  var QRMode = {
    MODE_NUMBER: 1,
    MODE_ALPHA_NUM: 2,
    MODE_8BIT_BYTE: 4,
    MODE_KANJI: 8
  }, QRErrorCorrectionLevel = {
    L: 1,
    M: 0,
    Q: 3,
    H: 2
  }, QRMaskPattern = {
    PATTERN000: 0,
    PATTERN001: 1,
    PATTERN010: 2,
    PATTERN011: 3,
    PATTERN100: 4,
    PATTERN101: 5,
    PATTERN110: 6,
    PATTERN111: 7
  }, QRUtil = (function() {
    let PATTERN_POSITION_TABLE = [
      [],
      [6, 18],
      [6, 22],
      [6, 26],
      [6, 30],
      [6, 34],
      [6, 22, 38],
      [6, 24, 42],
      [6, 26, 46],
      [6, 28, 50],
      [6, 30, 54],
      [6, 32, 58],
      [6, 34, 62],
      [6, 26, 46, 66],
      [6, 26, 48, 70],
      [6, 26, 50, 74],
      [6, 30, 54, 78],
      [6, 30, 56, 82],
      [6, 30, 58, 86],
      [6, 34, 62, 90],
      [6, 28, 50, 72, 94],
      [6, 26, 50, 74, 98],
      [6, 30, 54, 78, 102],
      [6, 28, 54, 80, 106],
      [6, 32, 58, 84, 110],
      [6, 30, 58, 86, 114],
      [6, 34, 62, 90, 118],
      [6, 26, 50, 74, 98, 122],
      [6, 30, 54, 78, 102, 126],
      [6, 26, 52, 78, 104, 130],
      [6, 30, 56, 82, 108, 134],
      [6, 34, 60, 86, 112, 138],
      [6, 30, 58, 86, 114, 142],
      [6, 34, 62, 90, 118, 146],
      [6, 30, 54, 78, 102, 126, 150],
      [6, 24, 50, 76, 102, 128, 154],
      [6, 28, 54, 80, 106, 132, 158],
      [6, 32, 58, 84, 110, 136, 162],
      [6, 26, 54, 82, 110, 138, 166],
      [6, 30, 58, 86, 114, 142, 170]
    ], G15 = 1335, G18 = 7973, G15_MASK = 21522, _this = {}, getBCHDigit = function(data) {
      let digit = 0;
      for (; data != 0; )
        digit += 1, data >>>= 1;
      return digit;
    };
    return _this.getBCHTypeInfo = function(data) {
      let d = data << 10;
      for (; getBCHDigit(d) - getBCHDigit(G15) >= 0; )
        d ^= G15 << getBCHDigit(d) - getBCHDigit(G15);
      return (data << 10 | d) ^ G15_MASK;
    }, _this.getBCHTypeNumber = function(data) {
      let d = data << 12;
      for (; getBCHDigit(d) - getBCHDigit(G18) >= 0; )
        d ^= G18 << getBCHDigit(d) - getBCHDigit(G18);
      return data << 12 | d;
    }, _this.getPatternPosition = function(typeNumber) {
      return PATTERN_POSITION_TABLE[typeNumber - 1];
    }, _this.getMaskFunction = function(maskPattern) {
      switch (maskPattern) {
        case QRMaskPattern.PATTERN000:
          return function(i, j) {
            return (i + j) % 2 == 0;
          };
        case QRMaskPattern.PATTERN001:
          return function(i, j) {
            return i % 2 == 0;
          };
        case QRMaskPattern.PATTERN010:
          return function(i, j) {
            return j % 3 == 0;
          };
        case QRMaskPattern.PATTERN011:
          return function(i, j) {
            return (i + j) % 3 == 0;
          };
        case QRMaskPattern.PATTERN100:
          return function(i, j) {
            return (Math.floor(i / 2) + Math.floor(j / 3)) % 2 == 0;
          };
        case QRMaskPattern.PATTERN101:
          return function(i, j) {
            return i * j % 2 + i * j % 3 == 0;
          };
        case QRMaskPattern.PATTERN110:
          return function(i, j) {
            return (i * j % 2 + i * j % 3) % 2 == 0;
          };
        case QRMaskPattern.PATTERN111:
          return function(i, j) {
            return (i * j % 3 + (i + j) % 2) % 2 == 0;
          };
        default:
          throw "bad maskPattern:" + maskPattern;
      }
    }, _this.getErrorCorrectPolynomial = function(errorCorrectLength) {
      let a = qrPolynomial([1], 0);
      for (let i = 0; i < errorCorrectLength; i += 1)
        a = a.multiply(qrPolynomial([1, QRMath.gexp(i)], 0));
      return a;
    }, _this.getLengthInBits = function(mode, type) {
      if (1 <= type && type < 10)
        switch (mode) {
          case QRMode.MODE_NUMBER:
            return 10;
          case QRMode.MODE_ALPHA_NUM:
            return 9;
          case QRMode.MODE_8BIT_BYTE:
            return 8;
          case QRMode.MODE_KANJI:
            return 8;
          default:
            throw "mode:" + mode;
        }
      else if (type < 27)
        switch (mode) {
          case QRMode.MODE_NUMBER:
            return 12;
          case QRMode.MODE_ALPHA_NUM:
            return 11;
          case QRMode.MODE_8BIT_BYTE:
            return 16;
          case QRMode.MODE_KANJI:
            return 10;
          default:
            throw "mode:" + mode;
        }
      else if (type < 41)
        switch (mode) {
          case QRMode.MODE_NUMBER:
            return 14;
          case QRMode.MODE_ALPHA_NUM:
            return 13;
          case QRMode.MODE_8BIT_BYTE:
            return 16;
          case QRMode.MODE_KANJI:
            return 12;
          default:
            throw "mode:" + mode;
        }
      else
        throw "type:" + type;
    }, _this.getLostPoint = function(qrcode2) {
      let moduleCount = qrcode2.getModuleCount(), lostPoint = 0;
      for (let row = 0; row < moduleCount; row += 1)
        for (let col = 0; col < moduleCount; col += 1) {
          let sameCount = 0, dark = qrcode2.isDark(row, col);
          for (let r = -1; r <= 1; r += 1)
            if (!(row + r < 0 || moduleCount <= row + r))
              for (let c = -1; c <= 1; c += 1)
                col + c < 0 || moduleCount <= col + c || r == 0 && c == 0 || dark == qrcode2.isDark(row + r, col + c) && (sameCount += 1);
          sameCount > 5 && (lostPoint += 3 + sameCount - 5);
        }
      for (let row = 0; row < moduleCount - 1; row += 1)
        for (let col = 0; col < moduleCount - 1; col += 1) {
          let count = 0;
          qrcode2.isDark(row, col) && (count += 1), qrcode2.isDark(row + 1, col) && (count += 1), qrcode2.isDark(row, col + 1) && (count += 1), qrcode2.isDark(row + 1, col + 1) && (count += 1), (count == 0 || count == 4) && (lostPoint += 3);
        }
      for (let row = 0; row < moduleCount; row += 1)
        for (let col = 0; col < moduleCount - 6; col += 1)
          qrcode2.isDark(row, col) && !qrcode2.isDark(row, col + 1) && qrcode2.isDark(row, col + 2) && qrcode2.isDark(row, col + 3) && qrcode2.isDark(row, col + 4) && !qrcode2.isDark(row, col + 5) && qrcode2.isDark(row, col + 6) && (lostPoint += 40);
      for (let col = 0; col < moduleCount; col += 1)
        for (let row = 0; row < moduleCount - 6; row += 1)
          qrcode2.isDark(row, col) && !qrcode2.isDark(row + 1, col) && qrcode2.isDark(row + 2, col) && qrcode2.isDark(row + 3, col) && qrcode2.isDark(row + 4, col) && !qrcode2.isDark(row + 5, col) && qrcode2.isDark(row + 6, col) && (lostPoint += 40);
      let darkCount = 0;
      for (let col = 0; col < moduleCount; col += 1)
        for (let row = 0; row < moduleCount; row += 1)
          qrcode2.isDark(row, col) && (darkCount += 1);
      let ratio = Math.abs(100 * darkCount / moduleCount / moduleCount - 50) / 5;
      return lostPoint += ratio * 10, lostPoint;
    }, _this;
  })(), QRMath = (function() {
    let EXP_TABLE = new Array(256), LOG_TABLE = new Array(256);
    for (let i = 0; i < 8; i += 1)
      EXP_TABLE[i] = 1 << i;
    for (let i = 8; i < 256; i += 1)
      EXP_TABLE[i] = EXP_TABLE[i - 4] ^ EXP_TABLE[i - 5] ^ EXP_TABLE[i - 6] ^ EXP_TABLE[i - 8];
    for (let i = 0; i < 255; i += 1)
      LOG_TABLE[EXP_TABLE[i]] = i;
    let _this = {};
    return _this.glog = function(n) {
      if (n < 1)
        throw "glog(" + n + ")";
      return LOG_TABLE[n];
    }, _this.gexp = function(n) {
      for (; n < 0; )
        n += 255;
      for (; n >= 256; )
        n -= 255;
      return EXP_TABLE[n];
    }, _this;
  })(), qrPolynomial = function(num, shift) {
    if (typeof num.length == "undefined")
      throw num.length + "/" + shift;
    let _num = (function() {
      let offset = 0;
      for (; offset < num.length && num[offset] == 0; )
        offset += 1;
      let _num2 = new Array(num.length - offset + shift);
      for (let i = 0; i < num.length - offset; i += 1)
        _num2[i] = num[i + offset];
      return _num2;
    })(), _this = {};
    return _this.getAt = function(index) {
      return _num[index];
    }, _this.getLength = function() {
      return _num.length;
    }, _this.multiply = function(e) {
      let num2 = new Array(_this.getLength() + e.getLength() - 1);
      for (let i = 0; i < _this.getLength(); i += 1)
        for (let j = 0; j < e.getLength(); j += 1)
          num2[i + j] ^= QRMath.gexp(QRMath.glog(_this.getAt(i)) + QRMath.glog(e.getAt(j)));
      return qrPolynomial(num2, 0);
    }, _this.mod = function(e) {
      if (_this.getLength() - e.getLength() < 0)
        return _this;
      let ratio = QRMath.glog(_this.getAt(0)) - QRMath.glog(e.getAt(0)), num2 = new Array(_this.getLength());
      for (let i = 0; i < _this.getLength(); i += 1)
        num2[i] = _this.getAt(i);
      for (let i = 0; i < e.getLength(); i += 1)
        num2[i] ^= QRMath.gexp(QRMath.glog(e.getAt(i)) + ratio);
      return qrPolynomial(num2, 0).mod(e);
    }, _this;
  }, QRRSBlock = (function() {
    let RS_BLOCK_TABLE = [
      // L
      // M
      // Q
      // H
      // 1
      [1, 26, 19],
      [1, 26, 16],
      [1, 26, 13],
      [1, 26, 9],
      // 2
      [1, 44, 34],
      [1, 44, 28],
      [1, 44, 22],
      [1, 44, 16],
      // 3
      [1, 70, 55],
      [1, 70, 44],
      [2, 35, 17],
      [2, 35, 13],
      // 4
      [1, 100, 80],
      [2, 50, 32],
      [2, 50, 24],
      [4, 25, 9],
      // 5
      [1, 134, 108],
      [2, 67, 43],
      [2, 33, 15, 2, 34, 16],
      [2, 33, 11, 2, 34, 12],
      // 6
      [2, 86, 68],
      [4, 43, 27],
      [4, 43, 19],
      [4, 43, 15],
      // 7
      [2, 98, 78],
      [4, 49, 31],
      [2, 32, 14, 4, 33, 15],
      [4, 39, 13, 1, 40, 14],
      // 8
      [2, 121, 97],
      [2, 60, 38, 2, 61, 39],
      [4, 40, 18, 2, 41, 19],
      [4, 40, 14, 2, 41, 15],
      // 9
      [2, 146, 116],
      [3, 58, 36, 2, 59, 37],
      [4, 36, 16, 4, 37, 17],
      [4, 36, 12, 4, 37, 13],
      // 10
      [2, 86, 68, 2, 87, 69],
      [4, 69, 43, 1, 70, 44],
      [6, 43, 19, 2, 44, 20],
      [6, 43, 15, 2, 44, 16],
      // 11
      [4, 101, 81],
      [1, 80, 50, 4, 81, 51],
      [4, 50, 22, 4, 51, 23],
      [3, 36, 12, 8, 37, 13],
      // 12
      [2, 116, 92, 2, 117, 93],
      [6, 58, 36, 2, 59, 37],
      [4, 46, 20, 6, 47, 21],
      [7, 42, 14, 4, 43, 15],
      // 13
      [4, 133, 107],
      [8, 59, 37, 1, 60, 38],
      [8, 44, 20, 4, 45, 21],
      [12, 33, 11, 4, 34, 12],
      // 14
      [3, 145, 115, 1, 146, 116],
      [4, 64, 40, 5, 65, 41],
      [11, 36, 16, 5, 37, 17],
      [11, 36, 12, 5, 37, 13],
      // 15
      [5, 109, 87, 1, 110, 88],
      [5, 65, 41, 5, 66, 42],
      [5, 54, 24, 7, 55, 25],
      [11, 36, 12, 7, 37, 13],
      // 16
      [5, 122, 98, 1, 123, 99],
      [7, 73, 45, 3, 74, 46],
      [15, 43, 19, 2, 44, 20],
      [3, 45, 15, 13, 46, 16],
      // 17
      [1, 135, 107, 5, 136, 108],
      [10, 74, 46, 1, 75, 47],
      [1, 50, 22, 15, 51, 23],
      [2, 42, 14, 17, 43, 15],
      // 18
      [5, 150, 120, 1, 151, 121],
      [9, 69, 43, 4, 70, 44],
      [17, 50, 22, 1, 51, 23],
      [2, 42, 14, 19, 43, 15],
      // 19
      [3, 141, 113, 4, 142, 114],
      [3, 70, 44, 11, 71, 45],
      [17, 47, 21, 4, 48, 22],
      [9, 39, 13, 16, 40, 14],
      // 20
      [3, 135, 107, 5, 136, 108],
      [3, 67, 41, 13, 68, 42],
      [15, 54, 24, 5, 55, 25],
      [15, 43, 15, 10, 44, 16],
      // 21
      [4, 144, 116, 4, 145, 117],
      [17, 68, 42],
      [17, 50, 22, 6, 51, 23],
      [19, 46, 16, 6, 47, 17],
      // 22
      [2, 139, 111, 7, 140, 112],
      [17, 74, 46],
      [7, 54, 24, 16, 55, 25],
      [34, 37, 13],
      // 23
      [4, 151, 121, 5, 152, 122],
      [4, 75, 47, 14, 76, 48],
      [11, 54, 24, 14, 55, 25],
      [16, 45, 15, 14, 46, 16],
      // 24
      [6, 147, 117, 4, 148, 118],
      [6, 73, 45, 14, 74, 46],
      [11, 54, 24, 16, 55, 25],
      [30, 46, 16, 2, 47, 17],
      // 25
      [8, 132, 106, 4, 133, 107],
      [8, 75, 47, 13, 76, 48],
      [7, 54, 24, 22, 55, 25],
      [22, 45, 15, 13, 46, 16],
      // 26
      [10, 142, 114, 2, 143, 115],
      [19, 74, 46, 4, 75, 47],
      [28, 50, 22, 6, 51, 23],
      [33, 46, 16, 4, 47, 17],
      // 27
      [8, 152, 122, 4, 153, 123],
      [22, 73, 45, 3, 74, 46],
      [8, 53, 23, 26, 54, 24],
      [12, 45, 15, 28, 46, 16],
      // 28
      [3, 147, 117, 10, 148, 118],
      [3, 73, 45, 23, 74, 46],
      [4, 54, 24, 31, 55, 25],
      [11, 45, 15, 31, 46, 16],
      // 29
      [7, 146, 116, 7, 147, 117],
      [21, 73, 45, 7, 74, 46],
      [1, 53, 23, 37, 54, 24],
      [19, 45, 15, 26, 46, 16],
      // 30
      [5, 145, 115, 10, 146, 116],
      [19, 75, 47, 10, 76, 48],
      [15, 54, 24, 25, 55, 25],
      [23, 45, 15, 25, 46, 16],
      // 31
      [13, 145, 115, 3, 146, 116],
      [2, 74, 46, 29, 75, 47],
      [42, 54, 24, 1, 55, 25],
      [23, 45, 15, 28, 46, 16],
      // 32
      [17, 145, 115],
      [10, 74, 46, 23, 75, 47],
      [10, 54, 24, 35, 55, 25],
      [19, 45, 15, 35, 46, 16],
      // 33
      [17, 145, 115, 1, 146, 116],
      [14, 74, 46, 21, 75, 47],
      [29, 54, 24, 19, 55, 25],
      [11, 45, 15, 46, 46, 16],
      // 34
      [13, 145, 115, 6, 146, 116],
      [14, 74, 46, 23, 75, 47],
      [44, 54, 24, 7, 55, 25],
      [59, 46, 16, 1, 47, 17],
      // 35
      [12, 151, 121, 7, 152, 122],
      [12, 75, 47, 26, 76, 48],
      [39, 54, 24, 14, 55, 25],
      [22, 45, 15, 41, 46, 16],
      // 36
      [6, 151, 121, 14, 152, 122],
      [6, 75, 47, 34, 76, 48],
      [46, 54, 24, 10, 55, 25],
      [2, 45, 15, 64, 46, 16],
      // 37
      [17, 152, 122, 4, 153, 123],
      [29, 74, 46, 14, 75, 47],
      [49, 54, 24, 10, 55, 25],
      [24, 45, 15, 46, 46, 16],
      // 38
      [4, 152, 122, 18, 153, 123],
      [13, 74, 46, 32, 75, 47],
      [48, 54, 24, 14, 55, 25],
      [42, 45, 15, 32, 46, 16],
      // 39
      [20, 147, 117, 4, 148, 118],
      [40, 75, 47, 7, 76, 48],
      [43, 54, 24, 22, 55, 25],
      [10, 45, 15, 67, 46, 16],
      // 40
      [19, 148, 118, 6, 149, 119],
      [18, 75, 47, 31, 76, 48],
      [34, 54, 24, 34, 55, 25],
      [20, 45, 15, 61, 46, 16]
    ], qrRSBlock = function(totalCount, dataCount) {
      let _this2 = {};
      return _this2.totalCount = totalCount, _this2.dataCount = dataCount, _this2;
    }, _this = {}, getRsBlockTable = function(typeNumber, errorCorrectionLevel) {
      switch (errorCorrectionLevel) {
        case QRErrorCorrectionLevel.L:
          return RS_BLOCK_TABLE[(typeNumber - 1) * 4 + 0];
        case QRErrorCorrectionLevel.M:
          return RS_BLOCK_TABLE[(typeNumber - 1) * 4 + 1];
        case QRErrorCorrectionLevel.Q:
          return RS_BLOCK_TABLE[(typeNumber - 1) * 4 + 2];
        case QRErrorCorrectionLevel.H:
          return RS_BLOCK_TABLE[(typeNumber - 1) * 4 + 3];
        default:
          return;
      }
    };
    return _this.getRSBlocks = function(typeNumber, errorCorrectionLevel) {
      let rsBlock = getRsBlockTable(typeNumber, errorCorrectionLevel);
      if (typeof rsBlock == "undefined")
        throw "bad rs block @ typeNumber:" + typeNumber + "/errorCorrectionLevel:" + errorCorrectionLevel;
      let length = rsBlock.length / 3, list = [];
      for (let i = 0; i < length; i += 1) {
        let count = rsBlock[i * 3 + 0], totalCount = rsBlock[i * 3 + 1], dataCount = rsBlock[i * 3 + 2];
        for (let j = 0; j < count; j += 1)
          list.push(qrRSBlock(totalCount, dataCount));
      }
      return list;
    }, _this;
  })(), qrBitBuffer = function() {
    let _buffer = [], _length = 0, _this = {};
    return _this.getBuffer = function() {
      return _buffer;
    }, _this.getAt = function(index) {
      let bufIndex = Math.floor(index / 8);
      return (_buffer[bufIndex] >>> 7 - index % 8 & 1) == 1;
    }, _this.put = function(num, length) {
      for (let i = 0; i < length; i += 1)
        _this.putBit((num >>> length - i - 1 & 1) == 1);
    }, _this.getLengthInBits = function() {
      return _length;
    }, _this.putBit = function(bit) {
      let bufIndex = Math.floor(_length / 8);
      _buffer.length <= bufIndex && _buffer.push(0), bit && (_buffer[bufIndex] |= 128 >>> _length % 8), _length += 1;
    }, _this;
  }, qrNumber = function(data) {
    let _mode = QRMode.MODE_NUMBER, _data = data, _this = {};
    _this.getMode = function() {
      return _mode;
    }, _this.getLength = function(buffer) {
      return _data.length;
    }, _this.write = function(buffer) {
      let data2 = _data, i = 0;
      for (; i + 2 < data2.length; )
        buffer.put(strToNum(data2.substring(i, i + 3)), 10), i += 3;
      i < data2.length && (data2.length - i == 1 ? buffer.put(strToNum(data2.substring(i, i + 1)), 4) : data2.length - i == 2 && buffer.put(strToNum(data2.substring(i, i + 2)), 7));
    };
    let strToNum = function(s) {
      let num = 0;
      for (let i = 0; i < s.length; i += 1)
        num = num * 10 + chatToNum(s.charAt(i));
      return num;
    }, chatToNum = function(c) {
      if ("0" <= c && c <= "9")
        return c.charCodeAt(0) - 48;
      throw "illegal char :" + c;
    };
    return _this;
  }, qrAlphaNum = function(data) {
    let _mode = QRMode.MODE_ALPHA_NUM, _data = data, _this = {};
    _this.getMode = function() {
      return _mode;
    }, _this.getLength = function(buffer) {
      return _data.length;
    }, _this.write = function(buffer) {
      let s = _data, i = 0;
      for (; i + 1 < s.length; )
        buffer.put(
          getCode(s.charAt(i)) * 45 + getCode(s.charAt(i + 1)),
          11
        ), i += 2;
      i < s.length && buffer.put(getCode(s.charAt(i)), 6);
    };
    let getCode = function(c) {
      if ("0" <= c && c <= "9")
        return c.charCodeAt(0) - 48;
      if ("A" <= c && c <= "Z")
        return c.charCodeAt(0) - 65 + 10;
      switch (c) {
        case " ":
          return 36;
        case "$":
          return 37;
        case "%":
          return 38;
        case "*":
          return 39;
        case "+":
          return 40;
        case "-":
          return 41;
        case ".":
          return 42;
        case "/":
          return 43;
        case ":":
          return 44;
        default:
          throw "illegal char :" + c;
      }
    };
    return _this;
  }, qr8BitByte = function(data) {
    let _mode = QRMode.MODE_8BIT_BYTE, _data = data, _bytes = qrcode.stringToBytes(data), _this = {};
    return _this.getMode = function() {
      return _mode;
    }, _this.getLength = function(buffer) {
      return _bytes.length;
    }, _this.write = function(buffer) {
      for (let i = 0; i < _bytes.length; i += 1)
        buffer.put(_bytes[i], 8);
    }, _this;
  }, qrKanji = function(data) {
    let _mode = QRMode.MODE_KANJI, _data = data, stringToBytes2 = qrcode.stringToBytes;
    (function(c, code) {
      let test = stringToBytes2(c);
      if (test.length != 2 || (test[0] << 8 | test[1]) != code)
        throw "sjis not supported.";
    })("\u53CB", 38726);
    let _bytes = stringToBytes2(data), _this = {};
    return _this.getMode = function() {
      return _mode;
    }, _this.getLength = function(buffer) {
      return ~~(_bytes.length / 2);
    }, _this.write = function(buffer) {
      let data2 = _bytes, i = 0;
      for (; i + 1 < data2.length; ) {
        let c = (255 & data2[i]) << 8 | 255 & data2[i + 1];
        if (33088 <= c && c <= 40956)
          c -= 33088;
        else if (57408 <= c && c <= 60351)
          c -= 49472;
        else
          throw "illegal char at " + (i + 1) + "/" + c;
        c = (c >>> 8 & 255) * 192 + (c & 255), buffer.put(c, 13), i += 2;
      }
      if (i < data2.length)
        throw "illegal char at " + (i + 1);
    }, _this;
  }, byteArrayOutputStream = function() {
    let _bytes = [], _this = {};
    return _this.writeByte = function(b) {
      _bytes.push(b & 255);
    }, _this.writeShort = function(i) {
      _this.writeByte(i), _this.writeByte(i >>> 8);
    }, _this.writeBytes = function(b, off, len) {
      off = off || 0, len = len || b.length;
      for (let i = 0; i < len; i += 1)
        _this.writeByte(b[i + off]);
    }, _this.writeString = function(s) {
      for (let i = 0; i < s.length; i += 1)
        _this.writeByte(s.charCodeAt(i));
    }, _this.toByteArray = function() {
      return _bytes;
    }, _this.toString = function() {
      let s = "";
      s += "[";
      for (let i = 0; i < _bytes.length; i += 1)
        i > 0 && (s += ","), s += _bytes[i];
      return s += "]", s;
    }, _this;
  }, base64EncodeOutputStream = function() {
    let _buffer = 0, _buflen = 0, _length = 0, _base64 = "", _this = {}, writeEncoded = function(b) {
      _base64 += String.fromCharCode(encode(b & 63));
    }, encode = function(n) {
      if (n < 0)
        throw "n:" + n;
      if (n < 26)
        return 65 + n;
      if (n < 52)
        return 97 + (n - 26);
      if (n < 62)
        return 48 + (n - 52);
      if (n == 62)
        return 43;
      if (n == 63)
        return 47;
      throw "n:" + n;
    };
    return _this.writeByte = function(n) {
      for (_buffer = _buffer << 8 | n & 255, _buflen += 8, _length += 1; _buflen >= 6; )
        writeEncoded(_buffer >>> _buflen - 6), _buflen -= 6;
    }, _this.flush = function() {
      if (_buflen > 0 && (writeEncoded(_buffer << 6 - _buflen), _buffer = 0, _buflen = 0), _length % 3 != 0) {
        let padlen = 3 - _length % 3;
        for (let i = 0; i < padlen; i += 1)
          _base64 += "=";
      }
    }, _this.toString = function() {
      return _base64;
    }, _this;
  }, base64DecodeInputStream = function(str) {
    let _str = str, _pos = 0, _buffer = 0, _buflen = 0, _this = {};
    _this.read = function() {
      for (; _buflen < 8; ) {
        if (_pos >= _str.length) {
          if (_buflen == 0)
            return -1;
          throw "unexpected end of file./" + _buflen;
        }
        let c = _str.charAt(_pos);
        if (_pos += 1, c == "=")
          return _buflen = 0, -1;
        if (c.match(/^\s$/))
          continue;
        _buffer = _buffer << 6 | decode(c.charCodeAt(0)), _buflen += 6;
      }
      let n = _buffer >>> _buflen - 8 & 255;
      return _buflen -= 8, n;
    };
    let decode = function(c) {
      if (65 <= c && c <= 90)
        return c - 65;
      if (97 <= c && c <= 122)
        return c - 97 + 26;
      if (48 <= c && c <= 57)
        return c - 48 + 52;
      if (c == 43)
        return 62;
      if (c == 47)
        return 63;
      throw "c:" + c;
    };
    return _this;
  }, gifImage = function(width, height) {
    let _width = width, _height = height, _data = new Array(width * height), _this = {};
    _this.setPixel = function(x, y, pixel) {
      _data[y * _width + x] = pixel;
    }, _this.write = function(out) {
      out.writeString("GIF87a"), out.writeShort(_width), out.writeShort(_height), out.writeByte(128), out.writeByte(0), out.writeByte(0), out.writeByte(0), out.writeByte(0), out.writeByte(0), out.writeByte(255), out.writeByte(255), out.writeByte(255), out.writeString(","), out.writeShort(0), out.writeShort(0), out.writeShort(_width), out.writeShort(_height), out.writeByte(0);
      let lzwMinCodeSize = 2, raster = getLZWRaster(lzwMinCodeSize);
      out.writeByte(lzwMinCodeSize);
      let offset = 0;
      for (; raster.length - offset > 255; )
        out.writeByte(255), out.writeBytes(raster, offset, 255), offset += 255;
      out.writeByte(raster.length - offset), out.writeBytes(raster, offset, raster.length - offset), out.writeByte(0), out.writeString(";");
    };
    let bitOutputStream = function(out) {
      let _out = out, _bitLength = 0, _bitBuffer = 0, _this2 = {};
      return _this2.write = function(data, length) {
        if (data >>> length)
          throw "length over";
        for (; _bitLength + length >= 8; )
          _out.writeByte(255 & (data << _bitLength | _bitBuffer)), length -= 8 - _bitLength, data >>>= 8 - _bitLength, _bitBuffer = 0, _bitLength = 0;
        _bitBuffer = data << _bitLength | _bitBuffer, _bitLength = _bitLength + length;
      }, _this2.flush = function() {
        _bitLength > 0 && _out.writeByte(_bitBuffer);
      }, _this2;
    }, getLZWRaster = function(lzwMinCodeSize) {
      let clearCode = 1 << lzwMinCodeSize, endCode = (1 << lzwMinCodeSize) + 1, bitLength = lzwMinCodeSize + 1, table = lzwTable();
      for (let i = 0; i < clearCode; i += 1)
        table.add(String.fromCharCode(i));
      table.add(String.fromCharCode(clearCode)), table.add(String.fromCharCode(endCode));
      let byteOut = byteArrayOutputStream(), bitOut = bitOutputStream(byteOut);
      bitOut.write(clearCode, bitLength);
      let dataIndex = 0, s = String.fromCharCode(_data[dataIndex]);
      for (dataIndex += 1; dataIndex < _data.length; ) {
        let c = String.fromCharCode(_data[dataIndex]);
        dataIndex += 1, table.contains(s + c) ? s = s + c : (bitOut.write(table.indexOf(s), bitLength), table.size() < 4095 && (table.size() == 1 << bitLength && (bitLength += 1), table.add(s + c)), s = c);
      }
      return bitOut.write(table.indexOf(s), bitLength), bitOut.write(endCode, bitLength), bitOut.flush(), byteOut.toByteArray();
    }, lzwTable = function() {
      let _map = {}, _size = 0, _this2 = {};
      return _this2.add = function(key) {
        if (_this2.contains(key))
          throw "dup key:" + key;
        _map[key] = _size, _size += 1;
      }, _this2.size = function() {
        return _size;
      }, _this2.indexOf = function(key) {
        return _map[key];
      }, _this2.contains = function(key) {
        return typeof _map[key] != "undefined";
      }, _this2;
    };
    return _this;
  }, createDataURL = function(width, height, getPixel) {
    let gif = gifImage(width, height);
    for (let y = 0; y < height; y += 1)
      for (let x = 0; x < width; x += 1)
        gif.setPixel(x, y, getPixel(x, y));
    let b = byteArrayOutputStream();
    gif.write(b);
    let base64 = base64EncodeOutputStream(), bytes = b.toByteArray();
    for (let i = 0; i < bytes.length; i += 1)
      base64.writeByte(bytes[i]);
    return base64.flush(), "data:image/gif;base64," + base64;
  }, qrcode_default = qrcode, stringToBytes = qrcode.stringToBytes;

  // src/xiaohongshu.ts
  function failure2(code, message) {
    return Object.assign(new Error(message), { code });
  }
  function requireStorage(window) {
    let api = window.xhs && window.xhs.miniTool;
    if (!api || typeof api.getStorageInfo != "function" || typeof api.getStorage != "function" || typeof api.setStorage != "function")
      throw failure2(
        "STORAGE_UNAVAILABLE",
        "\u52A0\u5BC6\u5B58\u50A8\u6682\u4E0D\u53EF\u7528\uFF0C\u8BF7\u91CD\u65B0\u6253\u5F00\u5C0F\u6E38\u620F\u540E\u91CD\u8BD5\uFF1B\u666E\u901A\u6E38\u620F\u4ECD\u53EF\u7EE7\u7EED\u3002"
      );
    return api;
  }
  function createXiaohongshuPlatform(window) {
    let host = window;
    return {
      async checkSupport() {
        let xhs = host.xhs, options = xhs && xhs.launchOptions, build = options && options.miniToolEnv && options.miniToolEnv.buildVersion;
        if (build == null || build === "") {
          let api = xhs && xhs.miniTool;
          if (api && typeof api.getLaunchOptions == "function")
            try {
              options = await api.getLaunchOptions(), build = options && options.miniToolEnv && options.miniToolEnv.buildVersion;
            } catch (e) {
              build = void 0;
            }
        }
        let version = typeof build == "number" || typeof build == "string" ? Number(build) : 0;
        if (!Number.isFinite(version) || Math.floor(version / 1e3) < 9460)
          throw failure2(
            "UNSUPPORTED_CLIENT",
            "\u8D2D\u4E70\u529F\u80FD\u9700\u8981\u5C0F\u7EA2\u4E66 9.46.0 \u6216\u66F4\u65B0\u7248\u672C\uFF0C\u8BF7\u5347\u7EA7\u540E\u91CD\u8BD5\uFF1B\u666E\u901A\u6E38\u620F\u4ECD\u53EF\u7EE7\u7EED\u3002"
          );
        requireStorage(host);
      },
      async read(key) {
        let api = requireStorage(host);
        try {
          let info = await api.getStorageInfo();
          if (!info || !Array.isArray(info.keys) || !info.keys.every((entry) => typeof entry == "string"))
            throw new Error("Invalid storage index");
          if (info.keys.indexOf(key) === -1) return null;
          let result = await api.getStorage({ key, encrypt: !0 });
          if (!result || result.data === void 0 || result.data === null)
            throw new Error("Missing existing storage value");
          return result.data;
        } catch (e) {
          throw failure2(
            "STORAGE_READ_FAILED",
            "\u65E0\u6CD5\u8BFB\u53D6\u8D2D\u4E70\u8BB0\u5F55\uFF0C\u8BF7\u91CD\u65B0\u6253\u5F00\u5C0F\u6E38\u620F\u540E\u91CD\u8BD5\u3002\u8BF7\u52FF\u6E05\u9664\u5B58\u50A8\uFF0C\u4EE5\u514D\u4E22\u5931\u5F53\u524D\u5B89\u88C5\u8BB0\u5F55\u3002"
          );
        }
      },
      async write(key, value) {
        let api = requireStorage(host);
        try {
          await api.setStorage({ key, data: value, encrypt: !0 });
        } catch (e) {
          throw failure2(
            "STORAGE_WRITE_FAILED",
            "\u8D2D\u4E70\u8BB0\u5F55\u4FDD\u5B58\u5931\u8D25\uFF0C\u8BF7\u68C0\u67E5\u5B58\u50A8\u7A7A\u95F4\u540E\u91CD\u8BD5\uFF1B\u52A8\u6001\u7801\u8FC7\u671F\u540E\u8BF7\u8FD4\u56DE\u4ED8\u6B3E\u9875\u9762\u5237\u65B0\u3002"
          );
        }
      },
      randomId() {
        try {
          if (!host.crypto || typeof host.crypto.getRandomValues != "function")
            throw new Error("Secure random unavailable");
          let bytes = new Uint8Array(16);
          host.crypto.getRandomValues(bytes), bytes[6] = bytes[6] & 15 | 64, bytes[8] = bytes[8] & 63 | 128;
          let id = "";
          for (let index = 0; index < bytes.length; index++)
            (index === 4 || index === 6 || index === 8 || index === 10) && (id += "-"), id += bytes[index].toString(16).padStart(2, "0");
          return id;
        } catch (e) {
          throw failure2(
            "RANDOM_UNAVAILABLE",
            "\u65E0\u6CD5\u5B89\u5168\u5EFA\u7ACB\u5B89\u88C5\u8BB0\u5F55\uFF0C\u8BF7\u91CD\u65B0\u6253\u5F00\u6216\u5347\u7EA7\u5C0F\u7EA2\u4E66\u540E\u91CD\u8BD5\u3002"
          );
        }
      },
      async saveCard(card) {
        let canvas;
        try {
          let api = host.xhs && host.xhs.miniTool;
          if (!api || typeof api.writeTempFile != "function" || typeof api.saveImageToPhotosAlbum != "function")
            throw new Error("Album API unavailable");
          canvas = drawCard(host.document, card);
          let temporary = await api.writeTempFile({
            data: canvas.toDataURL("image/png")
          });
          if (!temporary || typeof temporary.filePath != "string" || !temporary.filePath || /^https?:/i.test(temporary.filePath))
            throw new Error("Invalid temporary file");
          await api.saveImageToPhotosAlbum({ filePath: temporary.filePath });
        } catch (e) {
          throw failure2(
            "CARD_SAVE_FAILED",
            "\u4ED8\u6B3E\u5361\u4FDD\u5B58\u5931\u8D25\uFF0C\u8BF7\u5141\u8BB8\u76F8\u518C\u6743\u9650\uFF0C\u518D\u70B9\u51FB\u4FDD\u5B58\u91CD\u8BD5\u3002"
          );
        } finally {
          canvas && (canvas.width = 0, canvas.height = 0);
        }
      }
    };
  }
  function drawCard(document, card) {
    let qr = qrcode_default(0, "M");
    qr.addData(card.checkoutUrl, "Byte"), qr.make();
    let canvas = document.createElement("canvas");
    canvas.width = 900, canvas.height = 1200;
    let context = canvas.getContext("2d");
    if (!context) throw new Error("Canvas unavailable");
    context.fillStyle = "#ffffff", context.fillRect(0, 0, canvas.width, canvas.height), context.textAlign = "center", context.textBaseline = "middle";
    let sandbox = card.environment === "SANDBOX";
    context.fillStyle = sandbox ? "#9b241f" : "#152238", context.fillRect(0, 0, 900, 96), context.fillStyle = "#ffffff", context.font = "bold 34px sans-serif", context.fillText(
      sandbox ? "SANDBOX TEST ONLY" : "ViceMe \xB7 \u9053\u5177\u8D2D\u4E70",
      450,
      48
    ), context.fillStyle = "#152238", context.font = "bold 36px sans-serif", context.fillText(fitTitle(context, card.workTitle, 780), 450, 162), context.font = "30px sans-serif", context.fillText(fitTitle(context, card.itemTitle, 780), 450, 222);
    let count = qr.getModuleCount(), moduleSize = Math.floor(680 / (count + 8));
    if (moduleSize < 2) throw new Error("Checkout QR too large");
    let left = Math.floor((900 - count * moduleSize) / 2), top = 300 + 4 * moduleSize;
    context.fillStyle = "#000000";
    for (let row = 0; row < count; row++)
      for (let column = 0; column < count; column++)
        qr.isDark(row, column) && context.fillRect(
          left + column * moduleSize,
          top + row * moduleSize,
          moduleSize,
          moduleSize
        );
    return context.fillStyle = "#152238", context.font = "28px sans-serif", context.fillText(
      sandbox ? "\u4EC5\u7528\u4E8E\u6D4B\u8BD5\uFF0C\u4E0D\u4F1A\u4EA7\u751F\u771F\u5B9E\u4ED8\u6B3E" : "\u5728\u5FAE\u4FE1\u4E2D\u8BC6\u522B\u4E8C\u7EF4\u7801\uFF0C\u67E5\u770B\u8BE6\u60C5\u540E\u8D2D\u4E70",
      450,
      1030
    ), context.font = "24px sans-serif", context.fillText("\u6743\u76CA\u7ED1\u5B9A\u5F53\u524D\u6E38\u620F\u5B89\u88C5\uFF0C\u8BA2\u5355\u5C5E\u4E8E\u4ED8\u6B3E\u8D26\u53F7", 450, 1084), context.fillText("\u5B8C\u6210\u540E\u8FD4\u56DE\u6E38\u620F\uFF0C\u8F93\u5165\u516D\u4F4D\u52A8\u6001\u7801\u89E3\u9501", 450, 1130), canvas;
  }
  function fitTitle(context, value, width) {
    let characters = Array.from(
      value.slice(0, 512).replace(/[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/g, " ").trim()
    );
    characters.length && /^[\ud800-\udbff]$/.test(characters[characters.length - 1]) && characters.pop();
    let text = characters.join("");
    if (value.length <= 512 && context.measureText(text).width <= width)
      return text;
    for (; characters.length && context.measureText(characters.join("") + "\u2026").width > width; )
      characters.pop();
    return characters.join("") + "\u2026";
  }
  return __toCommonJS(commerce_exports);
})();
