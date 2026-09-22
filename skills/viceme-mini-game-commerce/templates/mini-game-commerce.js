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

  // ../../node_modules/.pnpm/jsqr@1.4.0/node_modules/jsqr/dist/jsQR.js
  var require_jsQR = __commonJS({
    "../../node_modules/.pnpm/jsqr@1.4.0/node_modules/jsqr/dist/jsQR.js"(exports, module) {
      (function(root, factory) {
        typeof exports == "object" && typeof module == "object" ? module.exports = factory() : typeof define == "function" && define.amd ? define([], factory) : typeof exports == "object" ? exports.jsQR = factory() : root.jsQR = factory();
      })(typeof self != "undefined" ? self : exports, function() {
        return (
          /******/
          (function(modules) {
            var installedModules = {};
            function __webpack_require__(moduleId) {
              if (installedModules[moduleId])
                return installedModules[moduleId].exports;
              var module2 = installedModules[moduleId] = {
                /******/
                i: moduleId,
                /******/
                l: !1,
                /******/
                exports: {}
                /******/
              };
              return modules[moduleId].call(module2.exports, module2, module2.exports, __webpack_require__), module2.l = !0, module2.exports;
            }
            return __webpack_require__.m = modules, __webpack_require__.c = installedModules, __webpack_require__.d = function(exports2, name, getter) {
              __webpack_require__.o(exports2, name) || Object.defineProperty(exports2, name, {
                /******/
                configurable: !1,
                /******/
                enumerable: !0,
                /******/
                get: getter
                /******/
              });
            }, __webpack_require__.n = function(module2) {
              var getter = module2 && module2.__esModule ? (
                /******/
                function() {
                  return module2.default;
                }
              ) : (
                /******/
                function() {
                  return module2;
                }
              );
              return __webpack_require__.d(getter, "a", getter), getter;
            }, __webpack_require__.o = function(object, property) {
              return Object.prototype.hasOwnProperty.call(object, property);
            }, __webpack_require__.p = "", __webpack_require__(__webpack_require__.s = 3);
          })([
            /* 0 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var BitMatrix = (
                /** @class */
                (function() {
                  function BitMatrix2(data, width) {
                    this.width = width, this.height = data.length / width, this.data = data;
                  }
                  return BitMatrix2.createEmpty = function(width, height) {
                    return new BitMatrix2(new Uint8ClampedArray(width * height), width);
                  }, BitMatrix2.prototype.get = function(x, y) {
                    return x < 0 || x >= this.width || y < 0 || y >= this.height ? !1 : !!this.data[y * this.width + x];
                  }, BitMatrix2.prototype.set = function(x, y, v) {
                    this.data[y * this.width + x] = v ? 1 : 0;
                  }, BitMatrix2.prototype.setRegion = function(left, top, width, height, v) {
                    for (var y = top; y < top + height; y++)
                      for (var x = left; x < left + width; x++)
                        this.set(x, y, !!v);
                  }, BitMatrix2;
                })()
              );
              exports2.BitMatrix = BitMatrix;
            }),
            /* 1 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var GenericGFPoly_1 = __webpack_require__(2);
              function addOrSubtractGF(a, b) {
                return a ^ b;
              }
              exports2.addOrSubtractGF = addOrSubtractGF;
              var GenericGF = (
                /** @class */
                (function() {
                  function GenericGF2(primitive, size, genBase) {
                    this.primitive = primitive, this.size = size, this.generatorBase = genBase, this.expTable = new Array(this.size), this.logTable = new Array(this.size);
                    for (var x = 1, i = 0; i < this.size; i++)
                      this.expTable[i] = x, x = x * 2, x >= this.size && (x = (x ^ this.primitive) & this.size - 1);
                    for (var i = 0; i < this.size - 1; i++)
                      this.logTable[this.expTable[i]] = i;
                    this.zero = new GenericGFPoly_1.default(this, Uint8ClampedArray.from([0])), this.one = new GenericGFPoly_1.default(this, Uint8ClampedArray.from([1]));
                  }
                  return GenericGF2.prototype.multiply = function(a, b) {
                    return a === 0 || b === 0 ? 0 : this.expTable[(this.logTable[a] + this.logTable[b]) % (this.size - 1)];
                  }, GenericGF2.prototype.inverse = function(a) {
                    if (a === 0)
                      throw new Error("Can't invert 0");
                    return this.expTable[this.size - this.logTable[a] - 1];
                  }, GenericGF2.prototype.buildMonomial = function(degree, coefficient) {
                    if (degree < 0)
                      throw new Error("Invalid monomial degree less than 0");
                    if (coefficient === 0)
                      return this.zero;
                    var coefficients = new Uint8ClampedArray(degree + 1);
                    return coefficients[0] = coefficient, new GenericGFPoly_1.default(this, coefficients);
                  }, GenericGF2.prototype.log = function(a) {
                    if (a === 0)
                      throw new Error("Can't take log(0)");
                    return this.logTable[a];
                  }, GenericGF2.prototype.exp = function(a) {
                    return this.expTable[a];
                  }, GenericGF2;
                })()
              );
              exports2.default = GenericGF;
            }),
            /* 2 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var GenericGF_1 = __webpack_require__(1), GenericGFPoly = (
                /** @class */
                (function() {
                  function GenericGFPoly2(field, coefficients) {
                    if (coefficients.length === 0)
                      throw new Error("No coefficients.");
                    this.field = field;
                    var coefficientsLength = coefficients.length;
                    if (coefficientsLength > 1 && coefficients[0] === 0) {
                      for (var firstNonZero = 1; firstNonZero < coefficientsLength && coefficients[firstNonZero] === 0; )
                        firstNonZero++;
                      if (firstNonZero === coefficientsLength)
                        this.coefficients = field.zero.coefficients;
                      else {
                        this.coefficients = new Uint8ClampedArray(coefficientsLength - firstNonZero);
                        for (var i = 0; i < this.coefficients.length; i++)
                          this.coefficients[i] = coefficients[firstNonZero + i];
                      }
                    } else
                      this.coefficients = coefficients;
                  }
                  return GenericGFPoly2.prototype.degree = function() {
                    return this.coefficients.length - 1;
                  }, GenericGFPoly2.prototype.isZero = function() {
                    return this.coefficients[0] === 0;
                  }, GenericGFPoly2.prototype.getCoefficient = function(degree) {
                    return this.coefficients[this.coefficients.length - 1 - degree];
                  }, GenericGFPoly2.prototype.addOrSubtract = function(other) {
                    var _a;
                    if (this.isZero())
                      return other;
                    if (other.isZero())
                      return this;
                    var smallerCoefficients = this.coefficients, largerCoefficients = other.coefficients;
                    smallerCoefficients.length > largerCoefficients.length && (_a = [largerCoefficients, smallerCoefficients], smallerCoefficients = _a[0], largerCoefficients = _a[1]);
                    for (var sumDiff = new Uint8ClampedArray(largerCoefficients.length), lengthDiff = largerCoefficients.length - smallerCoefficients.length, i = 0; i < lengthDiff; i++)
                      sumDiff[i] = largerCoefficients[i];
                    for (var i = lengthDiff; i < largerCoefficients.length; i++)
                      sumDiff[i] = GenericGF_1.addOrSubtractGF(smallerCoefficients[i - lengthDiff], largerCoefficients[i]);
                    return new GenericGFPoly2(this.field, sumDiff);
                  }, GenericGFPoly2.prototype.multiply = function(scalar) {
                    if (scalar === 0)
                      return this.field.zero;
                    if (scalar === 1)
                      return this;
                    for (var size = this.coefficients.length, product = new Uint8ClampedArray(size), i = 0; i < size; i++)
                      product[i] = this.field.multiply(this.coefficients[i], scalar);
                    return new GenericGFPoly2(this.field, product);
                  }, GenericGFPoly2.prototype.multiplyPoly = function(other) {
                    if (this.isZero() || other.isZero())
                      return this.field.zero;
                    for (var aCoefficients = this.coefficients, aLength = aCoefficients.length, bCoefficients = other.coefficients, bLength = bCoefficients.length, product = new Uint8ClampedArray(aLength + bLength - 1), i = 0; i < aLength; i++)
                      for (var aCoeff = aCoefficients[i], j = 0; j < bLength; j++)
                        product[i + j] = GenericGF_1.addOrSubtractGF(product[i + j], this.field.multiply(aCoeff, bCoefficients[j]));
                    return new GenericGFPoly2(this.field, product);
                  }, GenericGFPoly2.prototype.multiplyByMonomial = function(degree, coefficient) {
                    if (degree < 0)
                      throw new Error("Invalid degree less than 0");
                    if (coefficient === 0)
                      return this.field.zero;
                    for (var size = this.coefficients.length, product = new Uint8ClampedArray(size + degree), i = 0; i < size; i++)
                      product[i] = this.field.multiply(this.coefficients[i], coefficient);
                    return new GenericGFPoly2(this.field, product);
                  }, GenericGFPoly2.prototype.evaluateAt = function(a) {
                    var result = 0;
                    if (a === 0)
                      return this.getCoefficient(0);
                    var size = this.coefficients.length;
                    if (a === 1)
                      return this.coefficients.forEach(function(coefficient) {
                        result = GenericGF_1.addOrSubtractGF(result, coefficient);
                      }), result;
                    result = this.coefficients[0];
                    for (var i = 1; i < size; i++)
                      result = GenericGF_1.addOrSubtractGF(this.field.multiply(a, result), this.coefficients[i]);
                    return result;
                  }, GenericGFPoly2;
                })()
              );
              exports2.default = GenericGFPoly;
            }),
            /* 3 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var binarizer_1 = __webpack_require__(4), decoder_1 = __webpack_require__(5), extractor_1 = __webpack_require__(11), locator_1 = __webpack_require__(12);
              function scan(matrix) {
                var locations = locator_1.locate(matrix);
                if (!locations)
                  return null;
                for (var _i = 0, locations_1 = locations; _i < locations_1.length; _i++) {
                  var location_1 = locations_1[_i], extracted = extractor_1.extract(matrix, location_1), decoded = decoder_1.decode(extracted.matrix);
                  if (decoded)
                    return {
                      binaryData: decoded.bytes,
                      data: decoded.text,
                      chunks: decoded.chunks,
                      version: decoded.version,
                      location: {
                        topRightCorner: extracted.mappingFunction(location_1.dimension, 0),
                        topLeftCorner: extracted.mappingFunction(0, 0),
                        bottomRightCorner: extracted.mappingFunction(location_1.dimension, location_1.dimension),
                        bottomLeftCorner: extracted.mappingFunction(0, location_1.dimension),
                        topRightFinderPattern: location_1.topRight,
                        topLeftFinderPattern: location_1.topLeft,
                        bottomLeftFinderPattern: location_1.bottomLeft,
                        bottomRightAlignmentPattern: location_1.alignmentPattern
                      }
                    };
                }
                return null;
              }
              var defaultOptions = {
                inversionAttempts: "attemptBoth"
              };
              function jsQR2(data, width, height, providedOptions) {
                providedOptions === void 0 && (providedOptions = {});
                var options = defaultOptions;
                Object.keys(options || {}).forEach(function(opt) {
                  options[opt] = providedOptions[opt] || options[opt];
                });
                var shouldInvert = options.inversionAttempts === "attemptBoth" || options.inversionAttempts === "invertFirst", tryInvertedFirst = options.inversionAttempts === "onlyInvert" || options.inversionAttempts === "invertFirst", _a = binarizer_1.binarize(data, width, height, shouldInvert), binarized = _a.binarized, inverted = _a.inverted, result = scan(tryInvertedFirst ? inverted : binarized);
                return !result && (options.inversionAttempts === "attemptBoth" || options.inversionAttempts === "invertFirst") && (result = scan(tryInvertedFirst ? binarized : inverted)), result;
              }
              jsQR2.default = jsQR2, exports2.default = jsQR2;
            }),
            /* 4 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var BitMatrix_1 = __webpack_require__(0), REGION_SIZE = 8, MIN_DYNAMIC_RANGE = 24;
              function numBetween(value, min, max) {
                return value < min ? min : value > max ? max : value;
              }
              var Matrix = (
                /** @class */
                (function() {
                  function Matrix2(width, height) {
                    this.width = width, this.data = new Uint8ClampedArray(width * height);
                  }
                  return Matrix2.prototype.get = function(x, y) {
                    return this.data[y * this.width + x];
                  }, Matrix2.prototype.set = function(x, y, value) {
                    this.data[y * this.width + x] = value;
                  }, Matrix2;
                })()
              );
              function binarize(data, width, height, returnInverted) {
                if (data.length !== width * height * 4)
                  throw new Error("Malformed data passed to binarizer.");
                for (var greyscalePixels = new Matrix(width, height), x = 0; x < width; x++)
                  for (var y = 0; y < height; y++) {
                    var r = data[(y * width + x) * 4 + 0], g = data[(y * width + x) * 4 + 1], b = data[(y * width + x) * 4 + 2];
                    greyscalePixels.set(x, y, 0.2126 * r + 0.7152 * g + 0.0722 * b);
                  }
                for (var horizontalRegionCount = Math.ceil(width / REGION_SIZE), verticalRegionCount = Math.ceil(height / REGION_SIZE), blackPoints = new Matrix(horizontalRegionCount, verticalRegionCount), verticalRegion = 0; verticalRegion < verticalRegionCount; verticalRegion++)
                  for (var hortizontalRegion = 0; hortizontalRegion < horizontalRegionCount; hortizontalRegion++) {
                    for (var sum = 0, min = 1 / 0, max = 0, y = 0; y < REGION_SIZE; y++)
                      for (var x = 0; x < REGION_SIZE; x++) {
                        var pixelLumosity = greyscalePixels.get(hortizontalRegion * REGION_SIZE + x, verticalRegion * REGION_SIZE + y);
                        sum += pixelLumosity, min = Math.min(min, pixelLumosity), max = Math.max(max, pixelLumosity);
                      }
                    var average = sum / Math.pow(REGION_SIZE, 2);
                    if (max - min <= MIN_DYNAMIC_RANGE && (average = min / 2, verticalRegion > 0 && hortizontalRegion > 0)) {
                      var averageNeighborBlackPoint = (blackPoints.get(hortizontalRegion, verticalRegion - 1) + 2 * blackPoints.get(hortizontalRegion - 1, verticalRegion) + blackPoints.get(hortizontalRegion - 1, verticalRegion - 1)) / 4;
                      min < averageNeighborBlackPoint && (average = averageNeighborBlackPoint);
                    }
                    blackPoints.set(hortizontalRegion, verticalRegion, average);
                  }
                var binarized = BitMatrix_1.BitMatrix.createEmpty(width, height), inverted = null;
                returnInverted && (inverted = BitMatrix_1.BitMatrix.createEmpty(width, height));
                for (var verticalRegion = 0; verticalRegion < verticalRegionCount; verticalRegion++)
                  for (var hortizontalRegion = 0; hortizontalRegion < horizontalRegionCount; hortizontalRegion++) {
                    for (var left = numBetween(hortizontalRegion, 2, horizontalRegionCount - 3), top_1 = numBetween(verticalRegion, 2, verticalRegionCount - 3), sum = 0, xRegion = -2; xRegion <= 2; xRegion++)
                      for (var yRegion = -2; yRegion <= 2; yRegion++)
                        sum += blackPoints.get(left + xRegion, top_1 + yRegion);
                    for (var threshold = sum / 25, xRegion = 0; xRegion < REGION_SIZE; xRegion++)
                      for (var yRegion = 0; yRegion < REGION_SIZE; yRegion++) {
                        var x = hortizontalRegion * REGION_SIZE + xRegion, y = verticalRegion * REGION_SIZE + yRegion, lum = greyscalePixels.get(x, y);
                        binarized.set(x, y, lum <= threshold), returnInverted && inverted.set(x, y, !(lum <= threshold));
                      }
                  }
                return returnInverted ? { binarized, inverted } : { binarized };
              }
              exports2.binarize = binarize;
            }),
            /* 5 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var BitMatrix_1 = __webpack_require__(0), decodeData_1 = __webpack_require__(6), reedsolomon_1 = __webpack_require__(9), version_1 = __webpack_require__(10);
              function numBitsDiffering(x, y) {
                for (var z = x ^ y, bitCount = 0; z; )
                  bitCount++, z &= z - 1;
                return bitCount;
              }
              function pushBit(bit, byte) {
                return byte << 1 | bit;
              }
              var FORMAT_INFO_TABLE = [
                { bits: 21522, formatInfo: { errorCorrectionLevel: 1, dataMask: 0 } },
                { bits: 20773, formatInfo: { errorCorrectionLevel: 1, dataMask: 1 } },
                { bits: 24188, formatInfo: { errorCorrectionLevel: 1, dataMask: 2 } },
                { bits: 23371, formatInfo: { errorCorrectionLevel: 1, dataMask: 3 } },
                { bits: 17913, formatInfo: { errorCorrectionLevel: 1, dataMask: 4 } },
                { bits: 16590, formatInfo: { errorCorrectionLevel: 1, dataMask: 5 } },
                { bits: 20375, formatInfo: { errorCorrectionLevel: 1, dataMask: 6 } },
                { bits: 19104, formatInfo: { errorCorrectionLevel: 1, dataMask: 7 } },
                { bits: 30660, formatInfo: { errorCorrectionLevel: 0, dataMask: 0 } },
                { bits: 29427, formatInfo: { errorCorrectionLevel: 0, dataMask: 1 } },
                { bits: 32170, formatInfo: { errorCorrectionLevel: 0, dataMask: 2 } },
                { bits: 30877, formatInfo: { errorCorrectionLevel: 0, dataMask: 3 } },
                { bits: 26159, formatInfo: { errorCorrectionLevel: 0, dataMask: 4 } },
                { bits: 25368, formatInfo: { errorCorrectionLevel: 0, dataMask: 5 } },
                { bits: 27713, formatInfo: { errorCorrectionLevel: 0, dataMask: 6 } },
                { bits: 26998, formatInfo: { errorCorrectionLevel: 0, dataMask: 7 } },
                { bits: 5769, formatInfo: { errorCorrectionLevel: 3, dataMask: 0 } },
                { bits: 5054, formatInfo: { errorCorrectionLevel: 3, dataMask: 1 } },
                { bits: 7399, formatInfo: { errorCorrectionLevel: 3, dataMask: 2 } },
                { bits: 6608, formatInfo: { errorCorrectionLevel: 3, dataMask: 3 } },
                { bits: 1890, formatInfo: { errorCorrectionLevel: 3, dataMask: 4 } },
                { bits: 597, formatInfo: { errorCorrectionLevel: 3, dataMask: 5 } },
                { bits: 3340, formatInfo: { errorCorrectionLevel: 3, dataMask: 6 } },
                { bits: 2107, formatInfo: { errorCorrectionLevel: 3, dataMask: 7 } },
                { bits: 13663, formatInfo: { errorCorrectionLevel: 2, dataMask: 0 } },
                { bits: 12392, formatInfo: { errorCorrectionLevel: 2, dataMask: 1 } },
                { bits: 16177, formatInfo: { errorCorrectionLevel: 2, dataMask: 2 } },
                { bits: 14854, formatInfo: { errorCorrectionLevel: 2, dataMask: 3 } },
                { bits: 9396, formatInfo: { errorCorrectionLevel: 2, dataMask: 4 } },
                { bits: 8579, formatInfo: { errorCorrectionLevel: 2, dataMask: 5 } },
                { bits: 11994, formatInfo: { errorCorrectionLevel: 2, dataMask: 6 } },
                { bits: 11245, formatInfo: { errorCorrectionLevel: 2, dataMask: 7 } }
              ], DATA_MASKS = [
                function(p) {
                  return (p.y + p.x) % 2 === 0;
                },
                function(p) {
                  return p.y % 2 === 0;
                },
                function(p) {
                  return p.x % 3 === 0;
                },
                function(p) {
                  return (p.y + p.x) % 3 === 0;
                },
                function(p) {
                  return (Math.floor(p.y / 2) + Math.floor(p.x / 3)) % 2 === 0;
                },
                function(p) {
                  return p.x * p.y % 2 + p.x * p.y % 3 === 0;
                },
                function(p) {
                  return (p.y * p.x % 2 + p.y * p.x % 3) % 2 === 0;
                },
                function(p) {
                  return ((p.y + p.x) % 2 + p.y * p.x % 3) % 2 === 0;
                }
              ];
              function buildFunctionPatternMask(version) {
                var dimension = 17 + 4 * version.versionNumber, matrix = BitMatrix_1.BitMatrix.createEmpty(dimension, dimension);
                matrix.setRegion(0, 0, 9, 9, !0), matrix.setRegion(dimension - 8, 0, 8, 9, !0), matrix.setRegion(0, dimension - 8, 9, 8, !0);
                for (var _i = 0, _a = version.alignmentPatternCenters; _i < _a.length; _i++)
                  for (var x = _a[_i], _b = 0, _c = version.alignmentPatternCenters; _b < _c.length; _b++) {
                    var y = _c[_b];
                    x === 6 && y === 6 || x === 6 && y === dimension - 7 || x === dimension - 7 && y === 6 || matrix.setRegion(x - 2, y - 2, 5, 5, !0);
                  }
                return matrix.setRegion(6, 9, 1, dimension - 17, !0), matrix.setRegion(9, 6, dimension - 17, 1, !0), version.versionNumber > 6 && (matrix.setRegion(dimension - 11, 0, 3, 6, !0), matrix.setRegion(0, dimension - 11, 6, 3, !0)), matrix;
              }
              function readCodewords(matrix, version, formatInfo) {
                for (var dataMask = DATA_MASKS[formatInfo.dataMask], dimension = matrix.height, functionPatternMask = buildFunctionPatternMask(version), codewords = [], currentByte = 0, bitsRead = 0, readingUp = !0, columnIndex = dimension - 1; columnIndex > 0; columnIndex -= 2) {
                  columnIndex === 6 && columnIndex--;
                  for (var i = 0; i < dimension; i++)
                    for (var y = readingUp ? dimension - 1 - i : i, columnOffset = 0; columnOffset < 2; columnOffset++) {
                      var x = columnIndex - columnOffset;
                      if (!functionPatternMask.get(x, y)) {
                        bitsRead++;
                        var bit = matrix.get(x, y);
                        dataMask({ y, x }) && (bit = !bit), currentByte = pushBit(bit, currentByte), bitsRead === 8 && (codewords.push(currentByte), bitsRead = 0, currentByte = 0);
                      }
                    }
                  readingUp = !readingUp;
                }
                return codewords;
              }
              function readVersion(matrix) {
                var dimension = matrix.height, provisionalVersion = Math.floor((dimension - 17) / 4);
                if (provisionalVersion <= 6)
                  return version_1.VERSIONS[provisionalVersion - 1];
                for (var topRightVersionBits = 0, y = 5; y >= 0; y--)
                  for (var x = dimension - 9; x >= dimension - 11; x--)
                    topRightVersionBits = pushBit(matrix.get(x, y), topRightVersionBits);
                for (var bottomLeftVersionBits = 0, x = 5; x >= 0; x--)
                  for (var y = dimension - 9; y >= dimension - 11; y--)
                    bottomLeftVersionBits = pushBit(matrix.get(x, y), bottomLeftVersionBits);
                for (var bestDifference = 1 / 0, bestVersion, _i = 0, VERSIONS_1 = version_1.VERSIONS; _i < VERSIONS_1.length; _i++) {
                  var version = VERSIONS_1[_i];
                  if (version.infoBits === topRightVersionBits || version.infoBits === bottomLeftVersionBits)
                    return version;
                  var difference = numBitsDiffering(topRightVersionBits, version.infoBits);
                  difference < bestDifference && (bestVersion = version, bestDifference = difference), difference = numBitsDiffering(bottomLeftVersionBits, version.infoBits), difference < bestDifference && (bestVersion = version, bestDifference = difference);
                }
                if (bestDifference <= 3)
                  return bestVersion;
              }
              function readFormatInformation(matrix) {
                for (var topLeftFormatInfoBits = 0, x = 0; x <= 8; x++)
                  x !== 6 && (topLeftFormatInfoBits = pushBit(matrix.get(x, 8), topLeftFormatInfoBits));
                for (var y = 7; y >= 0; y--)
                  y !== 6 && (topLeftFormatInfoBits = pushBit(matrix.get(8, y), topLeftFormatInfoBits));
                for (var dimension = matrix.height, topRightBottomRightFormatInfoBits = 0, y = dimension - 1; y >= dimension - 7; y--)
                  topRightBottomRightFormatInfoBits = pushBit(matrix.get(8, y), topRightBottomRightFormatInfoBits);
                for (var x = dimension - 8; x < dimension; x++)
                  topRightBottomRightFormatInfoBits = pushBit(matrix.get(x, 8), topRightBottomRightFormatInfoBits);
                for (var bestDifference = 1 / 0, bestFormatInfo = null, _i = 0, FORMAT_INFO_TABLE_1 = FORMAT_INFO_TABLE; _i < FORMAT_INFO_TABLE_1.length; _i++) {
                  var _a = FORMAT_INFO_TABLE_1[_i], bits = _a.bits, formatInfo = _a.formatInfo;
                  if (bits === topLeftFormatInfoBits || bits === topRightBottomRightFormatInfoBits)
                    return formatInfo;
                  var difference = numBitsDiffering(topLeftFormatInfoBits, bits);
                  difference < bestDifference && (bestFormatInfo = formatInfo, bestDifference = difference), topLeftFormatInfoBits !== topRightBottomRightFormatInfoBits && (difference = numBitsDiffering(topRightBottomRightFormatInfoBits, bits), difference < bestDifference && (bestFormatInfo = formatInfo, bestDifference = difference));
                }
                return bestDifference <= 3 ? bestFormatInfo : null;
              }
              function getDataBlocks(codewords, version, ecLevel) {
                var ecInfo = version.errorCorrectionLevels[ecLevel], dataBlocks = [], totalCodewords = 0;
                if (ecInfo.ecBlocks.forEach(function(block) {
                  for (var i2 = 0; i2 < block.numBlocks; i2++)
                    dataBlocks.push({ numDataCodewords: block.dataCodewordsPerBlock, codewords: [] }), totalCodewords += block.dataCodewordsPerBlock + ecInfo.ecCodewordsPerBlock;
                }), codewords.length < totalCodewords)
                  return null;
                codewords = codewords.slice(0, totalCodewords);
                for (var shortBlockSize = ecInfo.ecBlocks[0].dataCodewordsPerBlock, i = 0; i < shortBlockSize; i++)
                  for (var _i = 0, dataBlocks_1 = dataBlocks; _i < dataBlocks_1.length; _i++) {
                    var dataBlock = dataBlocks_1[_i];
                    dataBlock.codewords.push(codewords.shift());
                  }
                if (ecInfo.ecBlocks.length > 1)
                  for (var smallBlockCount = ecInfo.ecBlocks[0].numBlocks, largeBlockCount = ecInfo.ecBlocks[1].numBlocks, i = 0; i < largeBlockCount; i++)
                    dataBlocks[smallBlockCount + i].codewords.push(codewords.shift());
                for (; codewords.length > 0; )
                  for (var _a = 0, dataBlocks_2 = dataBlocks; _a < dataBlocks_2.length; _a++) {
                    var dataBlock = dataBlocks_2[_a];
                    dataBlock.codewords.push(codewords.shift());
                  }
                return dataBlocks;
              }
              function decodeMatrix(matrix) {
                var version = readVersion(matrix);
                if (!version)
                  return null;
                var formatInfo = readFormatInformation(matrix);
                if (!formatInfo)
                  return null;
                var codewords = readCodewords(matrix, version, formatInfo), dataBlocks = getDataBlocks(codewords, version, formatInfo.errorCorrectionLevel);
                if (!dataBlocks)
                  return null;
                for (var totalBytes = dataBlocks.reduce(function(a, b) {
                  return a + b.numDataCodewords;
                }, 0), resultBytes = new Uint8ClampedArray(totalBytes), resultIndex = 0, _i = 0, dataBlocks_3 = dataBlocks; _i < dataBlocks_3.length; _i++) {
                  var dataBlock = dataBlocks_3[_i], correctedBytes = reedsolomon_1.decode(dataBlock.codewords, dataBlock.codewords.length - dataBlock.numDataCodewords);
                  if (!correctedBytes)
                    return null;
                  for (var i = 0; i < dataBlock.numDataCodewords; i++)
                    resultBytes[resultIndex++] = correctedBytes[i];
                }
                try {
                  return decodeData_1.decode(resultBytes, version.versionNumber);
                } catch (_a) {
                  return null;
                }
              }
              function decode(matrix) {
                if (matrix == null)
                  return null;
                var result = decodeMatrix(matrix);
                if (result)
                  return result;
                for (var x = 0; x < matrix.width; x++)
                  for (var y = x + 1; y < matrix.height; y++)
                    matrix.get(x, y) !== matrix.get(y, x) && (matrix.set(x, y, !matrix.get(x, y)), matrix.set(y, x, !matrix.get(y, x)));
                return decodeMatrix(matrix);
              }
              exports2.decode = decode;
            }),
            /* 6 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var BitStream_1 = __webpack_require__(7), shiftJISTable_1 = __webpack_require__(8), Mode;
              (function(Mode2) {
                Mode2.Numeric = "numeric", Mode2.Alphanumeric = "alphanumeric", Mode2.Byte = "byte", Mode2.Kanji = "kanji", Mode2.ECI = "eci";
              })(Mode = exports2.Mode || (exports2.Mode = {}));
              var ModeByte;
              (function(ModeByte2) {
                ModeByte2[ModeByte2.Terminator = 0] = "Terminator", ModeByte2[ModeByte2.Numeric = 1] = "Numeric", ModeByte2[ModeByte2.Alphanumeric = 2] = "Alphanumeric", ModeByte2[ModeByte2.Byte = 4] = "Byte", ModeByte2[ModeByte2.Kanji = 8] = "Kanji", ModeByte2[ModeByte2.ECI = 7] = "ECI";
              })(ModeByte || (ModeByte = {}));
              function decodeNumeric(stream, size) {
                for (var bytes = [], text = "", characterCountSize = [10, 12, 14][size], length = stream.readBits(characterCountSize); length >= 3; ) {
                  var num = stream.readBits(10);
                  if (num >= 1e3)
                    throw new Error("Invalid numeric value above 999");
                  var a = Math.floor(num / 100), b = Math.floor(num / 10) % 10, c = num % 10;
                  bytes.push(48 + a, 48 + b, 48 + c), text += a.toString() + b.toString() + c.toString(), length -= 3;
                }
                if (length === 2) {
                  var num = stream.readBits(7);
                  if (num >= 100)
                    throw new Error("Invalid numeric value above 99");
                  var a = Math.floor(num / 10), b = num % 10;
                  bytes.push(48 + a, 48 + b), text += a.toString() + b.toString();
                } else if (length === 1) {
                  var num = stream.readBits(4);
                  if (num >= 10)
                    throw new Error("Invalid numeric value above 9");
                  bytes.push(48 + num), text += num.toString();
                }
                return { bytes, text };
              }
              var AlphanumericCharacterCodes = [
                "0",
                "1",
                "2",
                "3",
                "4",
                "5",
                "6",
                "7",
                "8",
                "9",
                "A",
                "B",
                "C",
                "D",
                "E",
                "F",
                "G",
                "H",
                "I",
                "J",
                "K",
                "L",
                "M",
                "N",
                "O",
                "P",
                "Q",
                "R",
                "S",
                "T",
                "U",
                "V",
                "W",
                "X",
                "Y",
                "Z",
                " ",
                "$",
                "%",
                "*",
                "+",
                "-",
                ".",
                "/",
                ":"
              ];
              function decodeAlphanumeric(stream, size) {
                for (var bytes = [], text = "", characterCountSize = [9, 11, 13][size], length = stream.readBits(characterCountSize); length >= 2; ) {
                  var v = stream.readBits(11), a = Math.floor(v / 45), b = v % 45;
                  bytes.push(AlphanumericCharacterCodes[a].charCodeAt(0), AlphanumericCharacterCodes[b].charCodeAt(0)), text += AlphanumericCharacterCodes[a] + AlphanumericCharacterCodes[b], length -= 2;
                }
                if (length === 1) {
                  var a = stream.readBits(6);
                  bytes.push(AlphanumericCharacterCodes[a].charCodeAt(0)), text += AlphanumericCharacterCodes[a];
                }
                return { bytes, text };
              }
              function decodeByte(stream, size) {
                for (var bytes = [], text = "", characterCountSize = [8, 16, 16][size], length = stream.readBits(characterCountSize), i = 0; i < length; i++) {
                  var b = stream.readBits(8);
                  bytes.push(b);
                }
                try {
                  text += decodeURIComponent(bytes.map(function(b2) {
                    return "%" + ("0" + b2.toString(16)).substr(-2);
                  }).join(""));
                } catch (_a) {
                }
                return { bytes, text };
              }
              function decodeKanji(stream, size) {
                for (var bytes = [], text = "", characterCountSize = [8, 10, 12][size], length = stream.readBits(characterCountSize), i = 0; i < length; i++) {
                  var k = stream.readBits(13), c = Math.floor(k / 192) << 8 | k % 192;
                  c < 7936 ? c += 33088 : c += 49472, bytes.push(c >> 8, c & 255), text += String.fromCharCode(shiftJISTable_1.shiftJISTable[c]);
                }
                return { bytes, text };
              }
              function decode(data, version) {
                for (var _a, _b, _c, _d, stream = new BitStream_1.BitStream(data), size = version <= 9 ? 0 : version <= 26 ? 1 : 2, result = {
                  text: "",
                  bytes: [],
                  chunks: [],
                  version
                }; stream.available() >= 4; ) {
                  var mode = stream.readBits(4);
                  if (mode === ModeByte.Terminator)
                    return result;
                  if (mode === ModeByte.ECI)
                    stream.readBits(1) === 0 ? result.chunks.push({
                      type: Mode.ECI,
                      assignmentNumber: stream.readBits(7)
                    }) : stream.readBits(1) === 0 ? result.chunks.push({
                      type: Mode.ECI,
                      assignmentNumber: stream.readBits(14)
                    }) : stream.readBits(1) === 0 ? result.chunks.push({
                      type: Mode.ECI,
                      assignmentNumber: stream.readBits(21)
                    }) : result.chunks.push({
                      type: Mode.ECI,
                      assignmentNumber: -1
                    });
                  else if (mode === ModeByte.Numeric) {
                    var numericResult = decodeNumeric(stream, size);
                    result.text += numericResult.text, (_a = result.bytes).push.apply(_a, numericResult.bytes), result.chunks.push({
                      type: Mode.Numeric,
                      text: numericResult.text
                    });
                  } else if (mode === ModeByte.Alphanumeric) {
                    var alphanumericResult = decodeAlphanumeric(stream, size);
                    result.text += alphanumericResult.text, (_b = result.bytes).push.apply(_b, alphanumericResult.bytes), result.chunks.push({
                      type: Mode.Alphanumeric,
                      text: alphanumericResult.text
                    });
                  } else if (mode === ModeByte.Byte) {
                    var byteResult = decodeByte(stream, size);
                    result.text += byteResult.text, (_c = result.bytes).push.apply(_c, byteResult.bytes), result.chunks.push({
                      type: Mode.Byte,
                      bytes: byteResult.bytes,
                      text: byteResult.text
                    });
                  } else if (mode === ModeByte.Kanji) {
                    var kanjiResult = decodeKanji(stream, size);
                    result.text += kanjiResult.text, (_d = result.bytes).push.apply(_d, kanjiResult.bytes), result.chunks.push({
                      type: Mode.Kanji,
                      bytes: kanjiResult.bytes,
                      text: kanjiResult.text
                    });
                  }
                }
                if (stream.available() === 0 || stream.readBits(stream.available()) === 0)
                  return result;
              }
              exports2.decode = decode;
            }),
            /* 7 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var BitStream = (
                /** @class */
                (function() {
                  function BitStream2(bytes) {
                    this.byteOffset = 0, this.bitOffset = 0, this.bytes = bytes;
                  }
                  return BitStream2.prototype.readBits = function(numBits) {
                    if (numBits < 1 || numBits > 32 || numBits > this.available())
                      throw new Error("Cannot read " + numBits.toString() + " bits");
                    var result = 0;
                    if (this.bitOffset > 0) {
                      var bitsLeft = 8 - this.bitOffset, toRead = numBits < bitsLeft ? numBits : bitsLeft, bitsToNotRead = bitsLeft - toRead, mask = 255 >> 8 - toRead << bitsToNotRead;
                      result = (this.bytes[this.byteOffset] & mask) >> bitsToNotRead, numBits -= toRead, this.bitOffset += toRead, this.bitOffset === 8 && (this.bitOffset = 0, this.byteOffset++);
                    }
                    if (numBits > 0) {
                      for (; numBits >= 8; )
                        result = result << 8 | this.bytes[this.byteOffset] & 255, this.byteOffset++, numBits -= 8;
                      if (numBits > 0) {
                        var bitsToNotRead = 8 - numBits, mask = 255 >> bitsToNotRead << bitsToNotRead;
                        result = result << numBits | (this.bytes[this.byteOffset] & mask) >> bitsToNotRead, this.bitOffset += numBits;
                      }
                    }
                    return result;
                  }, BitStream2.prototype.available = function() {
                    return 8 * (this.bytes.length - this.byteOffset) - this.bitOffset;
                  }, BitStream2;
                })()
              );
              exports2.BitStream = BitStream;
            }),
            /* 8 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 }), exports2.shiftJISTable = {
                32: 32,
                33: 33,
                34: 34,
                35: 35,
                36: 36,
                37: 37,
                38: 38,
                39: 39,
                40: 40,
                41: 41,
                42: 42,
                43: 43,
                44: 44,
                45: 45,
                46: 46,
                47: 47,
                48: 48,
                49: 49,
                50: 50,
                51: 51,
                52: 52,
                53: 53,
                54: 54,
                55: 55,
                56: 56,
                57: 57,
                58: 58,
                59: 59,
                60: 60,
                61: 61,
                62: 62,
                63: 63,
                64: 64,
                65: 65,
                66: 66,
                67: 67,
                68: 68,
                69: 69,
                70: 70,
                71: 71,
                72: 72,
                73: 73,
                74: 74,
                75: 75,
                76: 76,
                77: 77,
                78: 78,
                79: 79,
                80: 80,
                81: 81,
                82: 82,
                83: 83,
                84: 84,
                85: 85,
                86: 86,
                87: 87,
                88: 88,
                89: 89,
                90: 90,
                91: 91,
                92: 165,
                93: 93,
                94: 94,
                95: 95,
                96: 96,
                97: 97,
                98: 98,
                99: 99,
                100: 100,
                101: 101,
                102: 102,
                103: 103,
                104: 104,
                105: 105,
                106: 106,
                107: 107,
                108: 108,
                109: 109,
                110: 110,
                111: 111,
                112: 112,
                113: 113,
                114: 114,
                115: 115,
                116: 116,
                117: 117,
                118: 118,
                119: 119,
                120: 120,
                121: 121,
                122: 122,
                123: 123,
                124: 124,
                125: 125,
                126: 8254,
                33088: 12288,
                33089: 12289,
                33090: 12290,
                33091: 65292,
                33092: 65294,
                33093: 12539,
                33094: 65306,
                33095: 65307,
                33096: 65311,
                33097: 65281,
                33098: 12443,
                33099: 12444,
                33100: 180,
                33101: 65344,
                33102: 168,
                33103: 65342,
                33104: 65507,
                33105: 65343,
                33106: 12541,
                33107: 12542,
                33108: 12445,
                33109: 12446,
                33110: 12291,
                33111: 20189,
                33112: 12293,
                33113: 12294,
                33114: 12295,
                33115: 12540,
                33116: 8213,
                33117: 8208,
                33118: 65295,
                33119: 92,
                33120: 12316,
                33121: 8214,
                33122: 65372,
                33123: 8230,
                33124: 8229,
                33125: 8216,
                33126: 8217,
                33127: 8220,
                33128: 8221,
                33129: 65288,
                33130: 65289,
                33131: 12308,
                33132: 12309,
                33133: 65339,
                33134: 65341,
                33135: 65371,
                33136: 65373,
                33137: 12296,
                33138: 12297,
                33139: 12298,
                33140: 12299,
                33141: 12300,
                33142: 12301,
                33143: 12302,
                33144: 12303,
                33145: 12304,
                33146: 12305,
                33147: 65291,
                33148: 8722,
                33149: 177,
                33150: 215,
                33152: 247,
                33153: 65309,
                33154: 8800,
                33155: 65308,
                33156: 65310,
                33157: 8806,
                33158: 8807,
                33159: 8734,
                33160: 8756,
                33161: 9794,
                33162: 9792,
                33163: 176,
                33164: 8242,
                33165: 8243,
                33166: 8451,
                33167: 65509,
                33168: 65284,
                33169: 162,
                33170: 163,
                33171: 65285,
                33172: 65283,
                33173: 65286,
                33174: 65290,
                33175: 65312,
                33176: 167,
                33177: 9734,
                33178: 9733,
                33179: 9675,
                33180: 9679,
                33181: 9678,
                33182: 9671,
                33183: 9670,
                33184: 9633,
                33185: 9632,
                33186: 9651,
                33187: 9650,
                33188: 9661,
                33189: 9660,
                33190: 8251,
                33191: 12306,
                33192: 8594,
                33193: 8592,
                33194: 8593,
                33195: 8595,
                33196: 12307,
                33208: 8712,
                33209: 8715,
                33210: 8838,
                33211: 8839,
                33212: 8834,
                33213: 8835,
                33214: 8746,
                33215: 8745,
                33224: 8743,
                33225: 8744,
                33226: 172,
                33227: 8658,
                33228: 8660,
                33229: 8704,
                33230: 8707,
                33242: 8736,
                33243: 8869,
                33244: 8978,
                33245: 8706,
                33246: 8711,
                33247: 8801,
                33248: 8786,
                33249: 8810,
                33250: 8811,
                33251: 8730,
                33252: 8765,
                33253: 8733,
                33254: 8757,
                33255: 8747,
                33256: 8748,
                33264: 8491,
                33265: 8240,
                33266: 9839,
                33267: 9837,
                33268: 9834,
                33269: 8224,
                33270: 8225,
                33271: 182,
                33276: 9711,
                33359: 65296,
                33360: 65297,
                33361: 65298,
                33362: 65299,
                33363: 65300,
                33364: 65301,
                33365: 65302,
                33366: 65303,
                33367: 65304,
                33368: 65305,
                33376: 65313,
                33377: 65314,
                33378: 65315,
                33379: 65316,
                33380: 65317,
                33381: 65318,
                33382: 65319,
                33383: 65320,
                33384: 65321,
                33385: 65322,
                33386: 65323,
                33387: 65324,
                33388: 65325,
                33389: 65326,
                33390: 65327,
                33391: 65328,
                33392: 65329,
                33393: 65330,
                33394: 65331,
                33395: 65332,
                33396: 65333,
                33397: 65334,
                33398: 65335,
                33399: 65336,
                33400: 65337,
                33401: 65338,
                33409: 65345,
                33410: 65346,
                33411: 65347,
                33412: 65348,
                33413: 65349,
                33414: 65350,
                33415: 65351,
                33416: 65352,
                33417: 65353,
                33418: 65354,
                33419: 65355,
                33420: 65356,
                33421: 65357,
                33422: 65358,
                33423: 65359,
                33424: 65360,
                33425: 65361,
                33426: 65362,
                33427: 65363,
                33428: 65364,
                33429: 65365,
                33430: 65366,
                33431: 65367,
                33432: 65368,
                33433: 65369,
                33434: 65370,
                33439: 12353,
                33440: 12354,
                33441: 12355,
                33442: 12356,
                33443: 12357,
                33444: 12358,
                33445: 12359,
                33446: 12360,
                33447: 12361,
                33448: 12362,
                33449: 12363,
                33450: 12364,
                33451: 12365,
                33452: 12366,
                33453: 12367,
                33454: 12368,
                33455: 12369,
                33456: 12370,
                33457: 12371,
                33458: 12372,
                33459: 12373,
                33460: 12374,
                33461: 12375,
                33462: 12376,
                33463: 12377,
                33464: 12378,
                33465: 12379,
                33466: 12380,
                33467: 12381,
                33468: 12382,
                33469: 12383,
                33470: 12384,
                33471: 12385,
                33472: 12386,
                33473: 12387,
                33474: 12388,
                33475: 12389,
                33476: 12390,
                33477: 12391,
                33478: 12392,
                33479: 12393,
                33480: 12394,
                33481: 12395,
                33482: 12396,
                33483: 12397,
                33484: 12398,
                33485: 12399,
                33486: 12400,
                33487: 12401,
                33488: 12402,
                33489: 12403,
                33490: 12404,
                33491: 12405,
                33492: 12406,
                33493: 12407,
                33494: 12408,
                33495: 12409,
                33496: 12410,
                33497: 12411,
                33498: 12412,
                33499: 12413,
                33500: 12414,
                33501: 12415,
                33502: 12416,
                33503: 12417,
                33504: 12418,
                33505: 12419,
                33506: 12420,
                33507: 12421,
                33508: 12422,
                33509: 12423,
                33510: 12424,
                33511: 12425,
                33512: 12426,
                33513: 12427,
                33514: 12428,
                33515: 12429,
                33516: 12430,
                33517: 12431,
                33518: 12432,
                33519: 12433,
                33520: 12434,
                33521: 12435,
                33600: 12449,
                33601: 12450,
                33602: 12451,
                33603: 12452,
                33604: 12453,
                33605: 12454,
                33606: 12455,
                33607: 12456,
                33608: 12457,
                33609: 12458,
                33610: 12459,
                33611: 12460,
                33612: 12461,
                33613: 12462,
                33614: 12463,
                33615: 12464,
                33616: 12465,
                33617: 12466,
                33618: 12467,
                33619: 12468,
                33620: 12469,
                33621: 12470,
                33622: 12471,
                33623: 12472,
                33624: 12473,
                33625: 12474,
                33626: 12475,
                33627: 12476,
                33628: 12477,
                33629: 12478,
                33630: 12479,
                33631: 12480,
                33632: 12481,
                33633: 12482,
                33634: 12483,
                33635: 12484,
                33636: 12485,
                33637: 12486,
                33638: 12487,
                33639: 12488,
                33640: 12489,
                33641: 12490,
                33642: 12491,
                33643: 12492,
                33644: 12493,
                33645: 12494,
                33646: 12495,
                33647: 12496,
                33648: 12497,
                33649: 12498,
                33650: 12499,
                33651: 12500,
                33652: 12501,
                33653: 12502,
                33654: 12503,
                33655: 12504,
                33656: 12505,
                33657: 12506,
                33658: 12507,
                33659: 12508,
                33660: 12509,
                33661: 12510,
                33662: 12511,
                33664: 12512,
                33665: 12513,
                33666: 12514,
                33667: 12515,
                33668: 12516,
                33669: 12517,
                33670: 12518,
                33671: 12519,
                33672: 12520,
                33673: 12521,
                33674: 12522,
                33675: 12523,
                33676: 12524,
                33677: 12525,
                33678: 12526,
                33679: 12527,
                33680: 12528,
                33681: 12529,
                33682: 12530,
                33683: 12531,
                33684: 12532,
                33685: 12533,
                33686: 12534,
                33695: 913,
                33696: 914,
                33697: 915,
                33698: 916,
                33699: 917,
                33700: 918,
                33701: 919,
                33702: 920,
                33703: 921,
                33704: 922,
                33705: 923,
                33706: 924,
                33707: 925,
                33708: 926,
                33709: 927,
                33710: 928,
                33711: 929,
                33712: 931,
                33713: 932,
                33714: 933,
                33715: 934,
                33716: 935,
                33717: 936,
                33718: 937,
                33727: 945,
                33728: 946,
                33729: 947,
                33730: 948,
                33731: 949,
                33732: 950,
                33733: 951,
                33734: 952,
                33735: 953,
                33736: 954,
                33737: 955,
                33738: 956,
                33739: 957,
                33740: 958,
                33741: 959,
                33742: 960,
                33743: 961,
                33744: 963,
                33745: 964,
                33746: 965,
                33747: 966,
                33748: 967,
                33749: 968,
                33750: 969,
                33856: 1040,
                33857: 1041,
                33858: 1042,
                33859: 1043,
                33860: 1044,
                33861: 1045,
                33862: 1025,
                33863: 1046,
                33864: 1047,
                33865: 1048,
                33866: 1049,
                33867: 1050,
                33868: 1051,
                33869: 1052,
                33870: 1053,
                33871: 1054,
                33872: 1055,
                33873: 1056,
                33874: 1057,
                33875: 1058,
                33876: 1059,
                33877: 1060,
                33878: 1061,
                33879: 1062,
                33880: 1063,
                33881: 1064,
                33882: 1065,
                33883: 1066,
                33884: 1067,
                33885: 1068,
                33886: 1069,
                33887: 1070,
                33888: 1071,
                33904: 1072,
                33905: 1073,
                33906: 1074,
                33907: 1075,
                33908: 1076,
                33909: 1077,
                33910: 1105,
                33911: 1078,
                33912: 1079,
                33913: 1080,
                33914: 1081,
                33915: 1082,
                33916: 1083,
                33917: 1084,
                33918: 1085,
                33920: 1086,
                33921: 1087,
                33922: 1088,
                33923: 1089,
                33924: 1090,
                33925: 1091,
                33926: 1092,
                33927: 1093,
                33928: 1094,
                33929: 1095,
                33930: 1096,
                33931: 1097,
                33932: 1098,
                33933: 1099,
                33934: 1100,
                33935: 1101,
                33936: 1102,
                33937: 1103,
                33951: 9472,
                33952: 9474,
                33953: 9484,
                33954: 9488,
                33955: 9496,
                33956: 9492,
                33957: 9500,
                33958: 9516,
                33959: 9508,
                33960: 9524,
                33961: 9532,
                33962: 9473,
                33963: 9475,
                33964: 9487,
                33965: 9491,
                33966: 9499,
                33967: 9495,
                33968: 9507,
                33969: 9523,
                33970: 9515,
                33971: 9531,
                33972: 9547,
                33973: 9504,
                33974: 9519,
                33975: 9512,
                33976: 9527,
                33977: 9535,
                33978: 9501,
                33979: 9520,
                33980: 9509,
                33981: 9528,
                33982: 9538,
                34975: 20124,
                34976: 21782,
                34977: 23043,
                34978: 38463,
                34979: 21696,
                34980: 24859,
                34981: 25384,
                34982: 23030,
                34983: 36898,
                34984: 33909,
                34985: 33564,
                34986: 31312,
                34987: 24746,
                34988: 25569,
                34989: 28197,
                34990: 26093,
                34991: 33894,
                34992: 33446,
                34993: 39925,
                34994: 26771,
                34995: 22311,
                34996: 26017,
                34997: 25201,
                34998: 23451,
                34999: 22992,
                35e3: 34427,
                35001: 39156,
                35002: 32098,
                35003: 32190,
                35004: 39822,
                35005: 25110,
                35006: 31903,
                35007: 34999,
                35008: 23433,
                35009: 24245,
                35010: 25353,
                35011: 26263,
                35012: 26696,
                35013: 38343,
                35014: 38797,
                35015: 26447,
                35016: 20197,
                35017: 20234,
                35018: 20301,
                35019: 20381,
                35020: 20553,
                35021: 22258,
                35022: 22839,
                35023: 22996,
                35024: 23041,
                35025: 23561,
                35026: 24799,
                35027: 24847,
                35028: 24944,
                35029: 26131,
                35030: 26885,
                35031: 28858,
                35032: 30031,
                35033: 30064,
                35034: 31227,
                35035: 32173,
                35036: 32239,
                35037: 32963,
                35038: 33806,
                35039: 34915,
                35040: 35586,
                35041: 36949,
                35042: 36986,
                35043: 21307,
                35044: 20117,
                35045: 20133,
                35046: 22495,
                35047: 32946,
                35048: 37057,
                35049: 30959,
                35050: 19968,
                35051: 22769,
                35052: 28322,
                35053: 36920,
                35054: 31282,
                35055: 33576,
                35056: 33419,
                35057: 39983,
                35058: 20801,
                35059: 21360,
                35060: 21693,
                35061: 21729,
                35062: 22240,
                35063: 23035,
                35064: 24341,
                35065: 39154,
                35066: 28139,
                35067: 32996,
                35068: 34093,
                35136: 38498,
                35137: 38512,
                35138: 38560,
                35139: 38907,
                35140: 21515,
                35141: 21491,
                35142: 23431,
                35143: 28879,
                35144: 32701,
                35145: 36802,
                35146: 38632,
                35147: 21359,
                35148: 40284,
                35149: 31418,
                35150: 19985,
                35151: 30867,
                35152: 33276,
                35153: 28198,
                35154: 22040,
                35155: 21764,
                35156: 27421,
                35157: 34074,
                35158: 39995,
                35159: 23013,
                35160: 21417,
                35161: 28006,
                35162: 29916,
                35163: 38287,
                35164: 22082,
                35165: 20113,
                35166: 36939,
                35167: 38642,
                35168: 33615,
                35169: 39180,
                35170: 21473,
                35171: 21942,
                35172: 23344,
                35173: 24433,
                35174: 26144,
                35175: 26355,
                35176: 26628,
                35177: 27704,
                35178: 27891,
                35179: 27945,
                35180: 29787,
                35181: 30408,
                35182: 31310,
                35183: 38964,
                35184: 33521,
                35185: 34907,
                35186: 35424,
                35187: 37613,
                35188: 28082,
                35189: 30123,
                35190: 30410,
                35191: 39365,
                35192: 24742,
                35193: 35585,
                35194: 36234,
                35195: 38322,
                35196: 27022,
                35197: 21421,
                35198: 20870,
                35200: 22290,
                35201: 22576,
                35202: 22852,
                35203: 23476,
                35204: 24310,
                35205: 24616,
                35206: 25513,
                35207: 25588,
                35208: 27839,
                35209: 28436,
                35210: 28814,
                35211: 28948,
                35212: 29017,
                35213: 29141,
                35214: 29503,
                35215: 32257,
                35216: 33398,
                35217: 33489,
                35218: 34199,
                35219: 36960,
                35220: 37467,
                35221: 40219,
                35222: 22633,
                35223: 26044,
                35224: 27738,
                35225: 29989,
                35226: 20985,
                35227: 22830,
                35228: 22885,
                35229: 24448,
                35230: 24540,
                35231: 25276,
                35232: 26106,
                35233: 27178,
                35234: 27431,
                35235: 27572,
                35236: 29579,
                35237: 32705,
                35238: 35158,
                35239: 40236,
                35240: 40206,
                35241: 40644,
                35242: 23713,
                35243: 27798,
                35244: 33659,
                35245: 20740,
                35246: 23627,
                35247: 25014,
                35248: 33222,
                35249: 26742,
                35250: 29281,
                35251: 20057,
                35252: 20474,
                35253: 21368,
                35254: 24681,
                35255: 28201,
                35256: 31311,
                35257: 38899,
                35258: 19979,
                35259: 21270,
                35260: 20206,
                35261: 20309,
                35262: 20285,
                35263: 20385,
                35264: 20339,
                35265: 21152,
                35266: 21487,
                35267: 22025,
                35268: 22799,
                35269: 23233,
                35270: 23478,
                35271: 23521,
                35272: 31185,
                35273: 26247,
                35274: 26524,
                35275: 26550,
                35276: 27468,
                35277: 27827,
                35278: 28779,
                35279: 29634,
                35280: 31117,
                35281: 31166,
                35282: 31292,
                35283: 31623,
                35284: 33457,
                35285: 33499,
                35286: 33540,
                35287: 33655,
                35288: 33775,
                35289: 33747,
                35290: 34662,
                35291: 35506,
                35292: 22057,
                35293: 36008,
                35294: 36838,
                35295: 36942,
                35296: 38686,
                35297: 34442,
                35298: 20420,
                35299: 23784,
                35300: 25105,
                35301: 29273,
                35302: 30011,
                35303: 33253,
                35304: 33469,
                35305: 34558,
                35306: 36032,
                35307: 38597,
                35308: 39187,
                35309: 39381,
                35310: 20171,
                35311: 20250,
                35312: 35299,
                35313: 22238,
                35314: 22602,
                35315: 22730,
                35316: 24315,
                35317: 24555,
                35318: 24618,
                35319: 24724,
                35320: 24674,
                35321: 25040,
                35322: 25106,
                35323: 25296,
                35324: 25913,
                35392: 39745,
                35393: 26214,
                35394: 26800,
                35395: 28023,
                35396: 28784,
                35397: 30028,
                35398: 30342,
                35399: 32117,
                35400: 33445,
                35401: 34809,
                35402: 38283,
                35403: 38542,
                35404: 35997,
                35405: 20977,
                35406: 21182,
                35407: 22806,
                35408: 21683,
                35409: 23475,
                35410: 23830,
                35411: 24936,
                35412: 27010,
                35413: 28079,
                35414: 30861,
                35415: 33995,
                35416: 34903,
                35417: 35442,
                35418: 37799,
                35419: 39608,
                35420: 28012,
                35421: 39336,
                35422: 34521,
                35423: 22435,
                35424: 26623,
                35425: 34510,
                35426: 37390,
                35427: 21123,
                35428: 22151,
                35429: 21508,
                35430: 24275,
                35431: 25313,
                35432: 25785,
                35433: 26684,
                35434: 26680,
                35435: 27579,
                35436: 29554,
                35437: 30906,
                35438: 31339,
                35439: 35226,
                35440: 35282,
                35441: 36203,
                35442: 36611,
                35443: 37101,
                35444: 38307,
                35445: 38548,
                35446: 38761,
                35447: 23398,
                35448: 23731,
                35449: 27005,
                35450: 38989,
                35451: 38990,
                35452: 25499,
                35453: 31520,
                35454: 27179,
                35456: 27263,
                35457: 26806,
                35458: 39949,
                35459: 28511,
                35460: 21106,
                35461: 21917,
                35462: 24688,
                35463: 25324,
                35464: 27963,
                35465: 28167,
                35466: 28369,
                35467: 33883,
                35468: 35088,
                35469: 36676,
                35470: 19988,
                35471: 39993,
                35472: 21494,
                35473: 26907,
                35474: 27194,
                35475: 38788,
                35476: 26666,
                35477: 20828,
                35478: 31427,
                35479: 33970,
                35480: 37340,
                35481: 37772,
                35482: 22107,
                35483: 40232,
                35484: 26658,
                35485: 33541,
                35486: 33841,
                35487: 31909,
                35488: 21e3,
                35489: 33477,
                35490: 29926,
                35491: 20094,
                35492: 20355,
                35493: 20896,
                35494: 23506,
                35495: 21002,
                35496: 21208,
                35497: 21223,
                35498: 24059,
                35499: 21914,
                35500: 22570,
                35501: 23014,
                35502: 23436,
                35503: 23448,
                35504: 23515,
                35505: 24178,
                35506: 24185,
                35507: 24739,
                35508: 24863,
                35509: 24931,
                35510: 25022,
                35511: 25563,
                35512: 25954,
                35513: 26577,
                35514: 26707,
                35515: 26874,
                35516: 27454,
                35517: 27475,
                35518: 27735,
                35519: 28450,
                35520: 28567,
                35521: 28485,
                35522: 29872,
                35523: 29976,
                35524: 30435,
                35525: 30475,
                35526: 31487,
                35527: 31649,
                35528: 31777,
                35529: 32233,
                35530: 32566,
                35531: 32752,
                35532: 32925,
                35533: 33382,
                35534: 33694,
                35535: 35251,
                35536: 35532,
                35537: 36011,
                35538: 36996,
                35539: 37969,
                35540: 38291,
                35541: 38289,
                35542: 38306,
                35543: 38501,
                35544: 38867,
                35545: 39208,
                35546: 33304,
                35547: 20024,
                35548: 21547,
                35549: 23736,
                35550: 24012,
                35551: 29609,
                35552: 30284,
                35553: 30524,
                35554: 23721,
                35555: 32747,
                35556: 36107,
                35557: 38593,
                35558: 38929,
                35559: 38996,
                35560: 39e3,
                35561: 20225,
                35562: 20238,
                35563: 21361,
                35564: 21916,
                35565: 22120,
                35566: 22522,
                35567: 22855,
                35568: 23305,
                35569: 23492,
                35570: 23696,
                35571: 24076,
                35572: 24190,
                35573: 24524,
                35574: 25582,
                35575: 26426,
                35576: 26071,
                35577: 26082,
                35578: 26399,
                35579: 26827,
                35580: 26820,
                35648: 27231,
                35649: 24112,
                35650: 27589,
                35651: 27671,
                35652: 27773,
                35653: 30079,
                35654: 31048,
                35655: 23395,
                35656: 31232,
                35657: 32e3,
                35658: 24509,
                35659: 35215,
                35660: 35352,
                35661: 36020,
                35662: 36215,
                35663: 36556,
                35664: 36637,
                35665: 39138,
                35666: 39438,
                35667: 39740,
                35668: 20096,
                35669: 20605,
                35670: 20736,
                35671: 22931,
                35672: 23452,
                35673: 25135,
                35674: 25216,
                35675: 25836,
                35676: 27450,
                35677: 29344,
                35678: 30097,
                35679: 31047,
                35680: 32681,
                35681: 34811,
                35682: 35516,
                35683: 35696,
                35684: 25516,
                35685: 33738,
                35686: 38816,
                35687: 21513,
                35688: 21507,
                35689: 21931,
                35690: 26708,
                35691: 27224,
                35692: 35440,
                35693: 30759,
                35694: 26485,
                35695: 40653,
                35696: 21364,
                35697: 23458,
                35698: 33050,
                35699: 34384,
                35700: 36870,
                35701: 19992,
                35702: 20037,
                35703: 20167,
                35704: 20241,
                35705: 21450,
                35706: 21560,
                35707: 23470,
                35708: 24339,
                35709: 24613,
                35710: 25937,
                35712: 26429,
                35713: 27714,
                35714: 27762,
                35715: 27875,
                35716: 28792,
                35717: 29699,
                35718: 31350,
                35719: 31406,
                35720: 31496,
                35721: 32026,
                35722: 31998,
                35723: 32102,
                35724: 26087,
                35725: 29275,
                35726: 21435,
                35727: 23621,
                35728: 24040,
                35729: 25298,
                35730: 25312,
                35731: 25369,
                35732: 28192,
                35733: 34394,
                35734: 35377,
                35735: 36317,
                35736: 37624,
                35737: 28417,
                35738: 31142,
                35739: 39770,
                35740: 20136,
                35741: 20139,
                35742: 20140,
                35743: 20379,
                35744: 20384,
                35745: 20689,
                35746: 20807,
                35747: 31478,
                35748: 20849,
                35749: 20982,
                35750: 21332,
                35751: 21281,
                35752: 21375,
                35753: 21483,
                35754: 21932,
                35755: 22659,
                35756: 23777,
                35757: 24375,
                35758: 24394,
                35759: 24623,
                35760: 24656,
                35761: 24685,
                35762: 25375,
                35763: 25945,
                35764: 27211,
                35765: 27841,
                35766: 29378,
                35767: 29421,
                35768: 30703,
                35769: 33016,
                35770: 33029,
                35771: 33288,
                35772: 34126,
                35773: 37111,
                35774: 37857,
                35775: 38911,
                35776: 39255,
                35777: 39514,
                35778: 20208,
                35779: 20957,
                35780: 23597,
                35781: 26241,
                35782: 26989,
                35783: 23616,
                35784: 26354,
                35785: 26997,
                35786: 29577,
                35787: 26704,
                35788: 31873,
                35789: 20677,
                35790: 21220,
                35791: 22343,
                35792: 24062,
                35793: 37670,
                35794: 26020,
                35795: 27427,
                35796: 27453,
                35797: 29748,
                35798: 31105,
                35799: 31165,
                35800: 31563,
                35801: 32202,
                35802: 33465,
                35803: 33740,
                35804: 34943,
                35805: 35167,
                35806: 35641,
                35807: 36817,
                35808: 37329,
                35809: 21535,
                35810: 37504,
                35811: 20061,
                35812: 20534,
                35813: 21477,
                35814: 21306,
                35815: 29399,
                35816: 29590,
                35817: 30697,
                35818: 33510,
                35819: 36527,
                35820: 39366,
                35821: 39368,
                35822: 39378,
                35823: 20855,
                35824: 24858,
                35825: 34398,
                35826: 21936,
                35827: 31354,
                35828: 20598,
                35829: 23507,
                35830: 36935,
                35831: 38533,
                35832: 20018,
                35833: 27355,
                35834: 37351,
                35835: 23633,
                35836: 23624,
                35904: 25496,
                35905: 31391,
                35906: 27795,
                35907: 38772,
                35908: 36705,
                35909: 31402,
                35910: 29066,
                35911: 38536,
                35912: 31874,
                35913: 26647,
                35914: 32368,
                35915: 26705,
                35916: 37740,
                35917: 21234,
                35918: 21531,
                35919: 34219,
                35920: 35347,
                35921: 32676,
                35922: 36557,
                35923: 37089,
                35924: 21350,
                35925: 34952,
                35926: 31041,
                35927: 20418,
                35928: 20670,
                35929: 21009,
                35930: 20804,
                35931: 21843,
                35932: 22317,
                35933: 29674,
                35934: 22411,
                35935: 22865,
                35936: 24418,
                35937: 24452,
                35938: 24693,
                35939: 24950,
                35940: 24935,
                35941: 25001,
                35942: 25522,
                35943: 25658,
                35944: 25964,
                35945: 26223,
                35946: 26690,
                35947: 28179,
                35948: 30054,
                35949: 31293,
                35950: 31995,
                35951: 32076,
                35952: 32153,
                35953: 32331,
                35954: 32619,
                35955: 33550,
                35956: 33610,
                35957: 34509,
                35958: 35336,
                35959: 35427,
                35960: 35686,
                35961: 36605,
                35962: 38938,
                35963: 40335,
                35964: 33464,
                35965: 36814,
                35966: 39912,
                35968: 21127,
                35969: 25119,
                35970: 25731,
                35971: 28608,
                35972: 38553,
                35973: 26689,
                35974: 20625,
                35975: 27424,
                35976: 27770,
                35977: 28500,
                35978: 31348,
                35979: 32080,
                35980: 34880,
                35981: 35363,
                35982: 26376,
                35983: 20214,
                35984: 20537,
                35985: 20518,
                35986: 20581,
                35987: 20860,
                35988: 21048,
                35989: 21091,
                35990: 21927,
                35991: 22287,
                35992: 22533,
                35993: 23244,
                35994: 24314,
                35995: 25010,
                35996: 25080,
                35997: 25331,
                35998: 25458,
                35999: 26908,
                36e3: 27177,
                36001: 29309,
                36002: 29356,
                36003: 29486,
                36004: 30740,
                36005: 30831,
                36006: 32121,
                36007: 30476,
                36008: 32937,
                36009: 35211,
                36010: 35609,
                36011: 36066,
                36012: 36562,
                36013: 36963,
                36014: 37749,
                36015: 38522,
                36016: 38997,
                36017: 39443,
                36018: 40568,
                36019: 20803,
                36020: 21407,
                36021: 21427,
                36022: 24187,
                36023: 24358,
                36024: 28187,
                36025: 28304,
                36026: 29572,
                36027: 29694,
                36028: 32067,
                36029: 33335,
                36030: 35328,
                36031: 35578,
                36032: 38480,
                36033: 20046,
                36034: 20491,
                36035: 21476,
                36036: 21628,
                36037: 22266,
                36038: 22993,
                36039: 23396,
                36040: 24049,
                36041: 24235,
                36042: 24359,
                36043: 25144,
                36044: 25925,
                36045: 26543,
                36046: 28246,
                36047: 29392,
                36048: 31946,
                36049: 34996,
                36050: 32929,
                36051: 32993,
                36052: 33776,
                36053: 34382,
                36054: 35463,
                36055: 36328,
                36056: 37431,
                36057: 38599,
                36058: 39015,
                36059: 40723,
                36060: 20116,
                36061: 20114,
                36062: 20237,
                36063: 21320,
                36064: 21577,
                36065: 21566,
                36066: 23087,
                36067: 24460,
                36068: 24481,
                36069: 24735,
                36070: 26791,
                36071: 27278,
                36072: 29786,
                36073: 30849,
                36074: 35486,
                36075: 35492,
                36076: 35703,
                36077: 37264,
                36078: 20062,
                36079: 39881,
                36080: 20132,
                36081: 20348,
                36082: 20399,
                36083: 20505,
                36084: 20502,
                36085: 20809,
                36086: 20844,
                36087: 21151,
                36088: 21177,
                36089: 21246,
                36090: 21402,
                36091: 21475,
                36092: 21521,
                36160: 21518,
                36161: 21897,
                36162: 22353,
                36163: 22434,
                36164: 22909,
                36165: 23380,
                36166: 23389,
                36167: 23439,
                36168: 24037,
                36169: 24039,
                36170: 24055,
                36171: 24184,
                36172: 24195,
                36173: 24218,
                36174: 24247,
                36175: 24344,
                36176: 24658,
                36177: 24908,
                36178: 25239,
                36179: 25304,
                36180: 25511,
                36181: 25915,
                36182: 26114,
                36183: 26179,
                36184: 26356,
                36185: 26477,
                36186: 26657,
                36187: 26775,
                36188: 27083,
                36189: 27743,
                36190: 27946,
                36191: 28009,
                36192: 28207,
                36193: 28317,
                36194: 30002,
                36195: 30343,
                36196: 30828,
                36197: 31295,
                36198: 31968,
                36199: 32005,
                36200: 32024,
                36201: 32094,
                36202: 32177,
                36203: 32789,
                36204: 32771,
                36205: 32943,
                36206: 32945,
                36207: 33108,
                36208: 33167,
                36209: 33322,
                36210: 33618,
                36211: 34892,
                36212: 34913,
                36213: 35611,
                36214: 36002,
                36215: 36092,
                36216: 37066,
                36217: 37237,
                36218: 37489,
                36219: 30783,
                36220: 37628,
                36221: 38308,
                36222: 38477,
                36224: 38917,
                36225: 39321,
                36226: 39640,
                36227: 40251,
                36228: 21083,
                36229: 21163,
                36230: 21495,
                36231: 21512,
                36232: 22741,
                36233: 25335,
                36234: 28640,
                36235: 35946,
                36236: 36703,
                36237: 40633,
                36238: 20811,
                36239: 21051,
                36240: 21578,
                36241: 22269,
                36242: 31296,
                36243: 37239,
                36244: 40288,
                36245: 40658,
                36246: 29508,
                36247: 28425,
                36248: 33136,
                36249: 29969,
                36250: 24573,
                36251: 24794,
                36252: 39592,
                36253: 29403,
                36254: 36796,
                36255: 27492,
                36256: 38915,
                36257: 20170,
                36258: 22256,
                36259: 22372,
                36260: 22718,
                36261: 23130,
                36262: 24680,
                36263: 25031,
                36264: 26127,
                36265: 26118,
                36266: 26681,
                36267: 26801,
                36268: 28151,
                36269: 30165,
                36270: 32058,
                36271: 33390,
                36272: 39746,
                36273: 20123,
                36274: 20304,
                36275: 21449,
                36276: 21766,
                36277: 23919,
                36278: 24038,
                36279: 24046,
                36280: 26619,
                36281: 27801,
                36282: 29811,
                36283: 30722,
                36284: 35408,
                36285: 37782,
                36286: 35039,
                36287: 22352,
                36288: 24231,
                36289: 25387,
                36290: 20661,
                36291: 20652,
                36292: 20877,
                36293: 26368,
                36294: 21705,
                36295: 22622,
                36296: 22971,
                36297: 23472,
                36298: 24425,
                36299: 25165,
                36300: 25505,
                36301: 26685,
                36302: 27507,
                36303: 28168,
                36304: 28797,
                36305: 37319,
                36306: 29312,
                36307: 30741,
                36308: 30758,
                36309: 31085,
                36310: 25998,
                36311: 32048,
                36312: 33756,
                36313: 35009,
                36314: 36617,
                36315: 38555,
                36316: 21092,
                36317: 22312,
                36318: 26448,
                36319: 32618,
                36320: 36001,
                36321: 20916,
                36322: 22338,
                36323: 38442,
                36324: 22586,
                36325: 27018,
                36326: 32948,
                36327: 21682,
                36328: 23822,
                36329: 22524,
                36330: 30869,
                36331: 40442,
                36332: 20316,
                36333: 21066,
                36334: 21643,
                36335: 25662,
                36336: 26152,
                36337: 26388,
                36338: 26613,
                36339: 31364,
                36340: 31574,
                36341: 32034,
                36342: 37679,
                36343: 26716,
                36344: 39853,
                36345: 31545,
                36346: 21273,
                36347: 20874,
                36348: 21047,
                36416: 23519,
                36417: 25334,
                36418: 25774,
                36419: 25830,
                36420: 26413,
                36421: 27578,
                36422: 34217,
                36423: 38609,
                36424: 30352,
                36425: 39894,
                36426: 25420,
                36427: 37638,
                36428: 39851,
                36429: 30399,
                36430: 26194,
                36431: 19977,
                36432: 20632,
                36433: 21442,
                36434: 23665,
                36435: 24808,
                36436: 25746,
                36437: 25955,
                36438: 26719,
                36439: 29158,
                36440: 29642,
                36441: 29987,
                36442: 31639,
                36443: 32386,
                36444: 34453,
                36445: 35715,
                36446: 36059,
                36447: 37240,
                36448: 39184,
                36449: 26028,
                36450: 26283,
                36451: 27531,
                36452: 20181,
                36453: 20180,
                36454: 20282,
                36455: 20351,
                36456: 21050,
                36457: 21496,
                36458: 21490,
                36459: 21987,
                36460: 22235,
                36461: 22763,
                36462: 22987,
                36463: 22985,
                36464: 23039,
                36465: 23376,
                36466: 23629,
                36467: 24066,
                36468: 24107,
                36469: 24535,
                36470: 24605,
                36471: 25351,
                36472: 25903,
                36473: 23388,
                36474: 26031,
                36475: 26045,
                36476: 26088,
                36477: 26525,
                36478: 27490,
                36480: 27515,
                36481: 27663,
                36482: 29509,
                36483: 31049,
                36484: 31169,
                36485: 31992,
                36486: 32025,
                36487: 32043,
                36488: 32930,
                36489: 33026,
                36490: 33267,
                36491: 35222,
                36492: 35422,
                36493: 35433,
                36494: 35430,
                36495: 35468,
                36496: 35566,
                36497: 36039,
                36498: 36060,
                36499: 38604,
                36500: 39164,
                36501: 27503,
                36502: 20107,
                36503: 20284,
                36504: 20365,
                36505: 20816,
                36506: 23383,
                36507: 23546,
                36508: 24904,
                36509: 25345,
                36510: 26178,
                36511: 27425,
                36512: 28363,
                36513: 27835,
                36514: 29246,
                36515: 29885,
                36516: 30164,
                36517: 30913,
                36518: 31034,
                36519: 32780,
                36520: 32819,
                36521: 33258,
                36522: 33940,
                36523: 36766,
                36524: 27728,
                36525: 40575,
                36526: 24335,
                36527: 35672,
                36528: 40235,
                36529: 31482,
                36530: 36600,
                36531: 23437,
                36532: 38635,
                36533: 19971,
                36534: 21489,
                36535: 22519,
                36536: 22833,
                36537: 23241,
                36538: 23460,
                36539: 24713,
                36540: 28287,
                36541: 28422,
                36542: 30142,
                36543: 36074,
                36544: 23455,
                36545: 34048,
                36546: 31712,
                36547: 20594,
                36548: 26612,
                36549: 33437,
                36550: 23649,
                36551: 34122,
                36552: 32286,
                36553: 33294,
                36554: 20889,
                36555: 23556,
                36556: 25448,
                36557: 36198,
                36558: 26012,
                36559: 29038,
                36560: 31038,
                36561: 32023,
                36562: 32773,
                36563: 35613,
                36564: 36554,
                36565: 36974,
                36566: 34503,
                36567: 37034,
                36568: 20511,
                36569: 21242,
                36570: 23610,
                36571: 26451,
                36572: 28796,
                36573: 29237,
                36574: 37196,
                36575: 37320,
                36576: 37675,
                36577: 33509,
                36578: 23490,
                36579: 24369,
                36580: 24825,
                36581: 20027,
                36582: 21462,
                36583: 23432,
                36584: 25163,
                36585: 26417,
                36586: 27530,
                36587: 29417,
                36588: 29664,
                36589: 31278,
                36590: 33131,
                36591: 36259,
                36592: 37202,
                36593: 39318,
                36594: 20754,
                36595: 21463,
                36596: 21610,
                36597: 23551,
                36598: 25480,
                36599: 27193,
                36600: 32172,
                36601: 38656,
                36602: 22234,
                36603: 21454,
                36604: 21608,
                36672: 23447,
                36673: 23601,
                36674: 24030,
                36675: 20462,
                36676: 24833,
                36677: 25342,
                36678: 27954,
                36679: 31168,
                36680: 31179,
                36681: 32066,
                36682: 32333,
                36683: 32722,
                36684: 33261,
                36685: 33311,
                36686: 33936,
                36687: 34886,
                36688: 35186,
                36689: 35728,
                36690: 36468,
                36691: 36655,
                36692: 36913,
                36693: 37195,
                36694: 37228,
                36695: 38598,
                36696: 37276,
                36697: 20160,
                36698: 20303,
                36699: 20805,
                36700: 21313,
                36701: 24467,
                36702: 25102,
                36703: 26580,
                36704: 27713,
                36705: 28171,
                36706: 29539,
                36707: 32294,
                36708: 37325,
                36709: 37507,
                36710: 21460,
                36711: 22809,
                36712: 23487,
                36713: 28113,
                36714: 31069,
                36715: 32302,
                36716: 31899,
                36717: 22654,
                36718: 29087,
                36719: 20986,
                36720: 34899,
                36721: 36848,
                36722: 20426,
                36723: 23803,
                36724: 26149,
                36725: 30636,
                36726: 31459,
                36727: 33308,
                36728: 39423,
                36729: 20934,
                36730: 24490,
                36731: 26092,
                36732: 26991,
                36733: 27529,
                36734: 28147,
                36736: 28310,
                36737: 28516,
                36738: 30462,
                36739: 32020,
                36740: 24033,
                36741: 36981,
                36742: 37255,
                36743: 38918,
                36744: 20966,
                36745: 21021,
                36746: 25152,
                36747: 26257,
                36748: 26329,
                36749: 28186,
                36750: 24246,
                36751: 32210,
                36752: 32626,
                36753: 26360,
                36754: 34223,
                36755: 34295,
                36756: 35576,
                36757: 21161,
                36758: 21465,
                36759: 22899,
                36760: 24207,
                36761: 24464,
                36762: 24661,
                36763: 37604,
                36764: 38500,
                36765: 20663,
                36766: 20767,
                36767: 21213,
                36768: 21280,
                36769: 21319,
                36770: 21484,
                36771: 21736,
                36772: 21830,
                36773: 21809,
                36774: 22039,
                36775: 22888,
                36776: 22974,
                36777: 23100,
                36778: 23477,
                36779: 23558,
                36780: 23567,
                36781: 23569,
                36782: 23578,
                36783: 24196,
                36784: 24202,
                36785: 24288,
                36786: 24432,
                36787: 25215,
                36788: 25220,
                36789: 25307,
                36790: 25484,
                36791: 25463,
                36792: 26119,
                36793: 26124,
                36794: 26157,
                36795: 26230,
                36796: 26494,
                36797: 26786,
                36798: 27167,
                36799: 27189,
                36800: 27836,
                36801: 28040,
                36802: 28169,
                36803: 28248,
                36804: 28988,
                36805: 28966,
                36806: 29031,
                36807: 30151,
                36808: 30465,
                36809: 30813,
                36810: 30977,
                36811: 31077,
                36812: 31216,
                36813: 31456,
                36814: 31505,
                36815: 31911,
                36816: 32057,
                36817: 32918,
                36818: 33750,
                36819: 33931,
                36820: 34121,
                36821: 34909,
                36822: 35059,
                36823: 35359,
                36824: 35388,
                36825: 35412,
                36826: 35443,
                36827: 35937,
                36828: 36062,
                36829: 37284,
                36830: 37478,
                36831: 37758,
                36832: 37912,
                36833: 38556,
                36834: 38808,
                36835: 19978,
                36836: 19976,
                36837: 19998,
                36838: 20055,
                36839: 20887,
                36840: 21104,
                36841: 22478,
                36842: 22580,
                36843: 22732,
                36844: 23330,
                36845: 24120,
                36846: 24773,
                36847: 25854,
                36848: 26465,
                36849: 26454,
                36850: 27972,
                36851: 29366,
                36852: 30067,
                36853: 31331,
                36854: 33976,
                36855: 35698,
                36856: 37304,
                36857: 37664,
                36858: 22065,
                36859: 22516,
                36860: 39166,
                36928: 25325,
                36929: 26893,
                36930: 27542,
                36931: 29165,
                36932: 32340,
                36933: 32887,
                36934: 33394,
                36935: 35302,
                36936: 39135,
                36937: 34645,
                36938: 36785,
                36939: 23611,
                36940: 20280,
                36941: 20449,
                36942: 20405,
                36943: 21767,
                36944: 23072,
                36945: 23517,
                36946: 23529,
                36947: 24515,
                36948: 24910,
                36949: 25391,
                36950: 26032,
                36951: 26187,
                36952: 26862,
                36953: 27035,
                36954: 28024,
                36955: 28145,
                36956: 30003,
                36957: 30137,
                36958: 30495,
                36959: 31070,
                36960: 31206,
                36961: 32051,
                36962: 33251,
                36963: 33455,
                36964: 34218,
                36965: 35242,
                36966: 35386,
                36967: 36523,
                36968: 36763,
                36969: 36914,
                36970: 37341,
                36971: 38663,
                36972: 20154,
                36973: 20161,
                36974: 20995,
                36975: 22645,
                36976: 22764,
                36977: 23563,
                36978: 29978,
                36979: 23613,
                36980: 33102,
                36981: 35338,
                36982: 36805,
                36983: 38499,
                36984: 38765,
                36985: 31525,
                36986: 35535,
                36987: 38920,
                36988: 37218,
                36989: 22259,
                36990: 21416,
                36992: 36887,
                36993: 21561,
                36994: 22402,
                36995: 24101,
                36996: 25512,
                36997: 27700,
                36998: 28810,
                36999: 30561,
                37e3: 31883,
                37001: 32736,
                37002: 34928,
                37003: 36930,
                37004: 37204,
                37005: 37648,
                37006: 37656,
                37007: 38543,
                37008: 29790,
                37009: 39620,
                37010: 23815,
                37011: 23913,
                37012: 25968,
                37013: 26530,
                37014: 36264,
                37015: 38619,
                37016: 25454,
                37017: 26441,
                37018: 26905,
                37019: 33733,
                37020: 38935,
                37021: 38592,
                37022: 35070,
                37023: 28548,
                37024: 25722,
                37025: 23544,
                37026: 19990,
                37027: 28716,
                37028: 30045,
                37029: 26159,
                37030: 20932,
                37031: 21046,
                37032: 21218,
                37033: 22995,
                37034: 24449,
                37035: 24615,
                37036: 25104,
                37037: 25919,
                37038: 25972,
                37039: 26143,
                37040: 26228,
                37041: 26866,
                37042: 26646,
                37043: 27491,
                37044: 28165,
                37045: 29298,
                37046: 29983,
                37047: 30427,
                37048: 31934,
                37049: 32854,
                37050: 22768,
                37051: 35069,
                37052: 35199,
                37053: 35488,
                37054: 35475,
                37055: 35531,
                37056: 36893,
                37057: 37266,
                37058: 38738,
                37059: 38745,
                37060: 25993,
                37061: 31246,
                37062: 33030,
                37063: 38587,
                37064: 24109,
                37065: 24796,
                37066: 25114,
                37067: 26021,
                37068: 26132,
                37069: 26512,
                37070: 30707,
                37071: 31309,
                37072: 31821,
                37073: 32318,
                37074: 33034,
                37075: 36012,
                37076: 36196,
                37077: 36321,
                37078: 36447,
                37079: 30889,
                37080: 20999,
                37081: 25305,
                37082: 25509,
                37083: 25666,
                37084: 25240,
                37085: 35373,
                37086: 31363,
                37087: 31680,
                37088: 35500,
                37089: 38634,
                37090: 32118,
                37091: 33292,
                37092: 34633,
                37093: 20185,
                37094: 20808,
                37095: 21315,
                37096: 21344,
                37097: 23459,
                37098: 23554,
                37099: 23574,
                37100: 24029,
                37101: 25126,
                37102: 25159,
                37103: 25776,
                37104: 26643,
                37105: 26676,
                37106: 27849,
                37107: 27973,
                37108: 27927,
                37109: 26579,
                37110: 28508,
                37111: 29006,
                37112: 29053,
                37113: 26059,
                37114: 31359,
                37115: 31661,
                37116: 32218,
                37184: 32330,
                37185: 32680,
                37186: 33146,
                37187: 33307,
                37188: 33337,
                37189: 34214,
                37190: 35438,
                37191: 36046,
                37192: 36341,
                37193: 36984,
                37194: 36983,
                37195: 37549,
                37196: 37521,
                37197: 38275,
                37198: 39854,
                37199: 21069,
                37200: 21892,
                37201: 28472,
                37202: 28982,
                37203: 20840,
                37204: 31109,
                37205: 32341,
                37206: 33203,
                37207: 31950,
                37208: 22092,
                37209: 22609,
                37210: 23720,
                37211: 25514,
                37212: 26366,
                37213: 26365,
                37214: 26970,
                37215: 29401,
                37216: 30095,
                37217: 30094,
                37218: 30990,
                37219: 31062,
                37220: 31199,
                37221: 31895,
                37222: 32032,
                37223: 32068,
                37224: 34311,
                37225: 35380,
                37226: 38459,
                37227: 36961,
                37228: 40736,
                37229: 20711,
                37230: 21109,
                37231: 21452,
                37232: 21474,
                37233: 20489,
                37234: 21930,
                37235: 22766,
                37236: 22863,
                37237: 29245,
                37238: 23435,
                37239: 23652,
                37240: 21277,
                37241: 24803,
                37242: 24819,
                37243: 25436,
                37244: 25475,
                37245: 25407,
                37246: 25531,
                37248: 25805,
                37249: 26089,
                37250: 26361,
                37251: 24035,
                37252: 27085,
                37253: 27133,
                37254: 28437,
                37255: 29157,
                37256: 20105,
                37257: 30185,
                37258: 30456,
                37259: 31379,
                37260: 31967,
                37261: 32207,
                37262: 32156,
                37263: 32865,
                37264: 33609,
                37265: 33624,
                37266: 33900,
                37267: 33980,
                37268: 34299,
                37269: 35013,
                37270: 36208,
                37271: 36865,
                37272: 36973,
                37273: 37783,
                37274: 38684,
                37275: 39442,
                37276: 20687,
                37277: 22679,
                37278: 24974,
                37279: 33235,
                37280: 34101,
                37281: 36104,
                37282: 36896,
                37283: 20419,
                37284: 20596,
                37285: 21063,
                37286: 21363,
                37287: 24687,
                37288: 25417,
                37289: 26463,
                37290: 28204,
                37291: 36275,
                37292: 36895,
                37293: 20439,
                37294: 23646,
                37295: 36042,
                37296: 26063,
                37297: 32154,
                37298: 21330,
                37299: 34966,
                37300: 20854,
                37301: 25539,
                37302: 23384,
                37303: 23403,
                37304: 23562,
                37305: 25613,
                37306: 26449,
                37307: 36956,
                37308: 20182,
                37309: 22810,
                37310: 22826,
                37311: 27760,
                37312: 35409,
                37313: 21822,
                37314: 22549,
                37315: 22949,
                37316: 24816,
                37317: 25171,
                37318: 26561,
                37319: 33333,
                37320: 26965,
                37321: 38464,
                37322: 39364,
                37323: 39464,
                37324: 20307,
                37325: 22534,
                37326: 23550,
                37327: 32784,
                37328: 23729,
                37329: 24111,
                37330: 24453,
                37331: 24608,
                37332: 24907,
                37333: 25140,
                37334: 26367,
                37335: 27888,
                37336: 28382,
                37337: 32974,
                37338: 33151,
                37339: 33492,
                37340: 34955,
                37341: 36024,
                37342: 36864,
                37343: 36910,
                37344: 38538,
                37345: 40667,
                37346: 39899,
                37347: 20195,
                37348: 21488,
                37349: 22823,
                37350: 31532,
                37351: 37261,
                37352: 38988,
                37353: 40441,
                37354: 28381,
                37355: 28711,
                37356: 21331,
                37357: 21828,
                37358: 23429,
                37359: 25176,
                37360: 25246,
                37361: 25299,
                37362: 27810,
                37363: 28655,
                37364: 29730,
                37365: 35351,
                37366: 37944,
                37367: 28609,
                37368: 35582,
                37369: 33592,
                37370: 20967,
                37371: 34552,
                37372: 21482,
                37440: 21481,
                37441: 20294,
                37442: 36948,
                37443: 36784,
                37444: 22890,
                37445: 33073,
                37446: 24061,
                37447: 31466,
                37448: 36799,
                37449: 26842,
                37450: 35895,
                37451: 29432,
                37452: 40008,
                37453: 27197,
                37454: 35504,
                37455: 20025,
                37456: 21336,
                37457: 22022,
                37458: 22374,
                37459: 25285,
                37460: 25506,
                37461: 26086,
                37462: 27470,
                37463: 28129,
                37464: 28251,
                37465: 28845,
                37466: 30701,
                37467: 31471,
                37468: 31658,
                37469: 32187,
                37470: 32829,
                37471: 32966,
                37472: 34507,
                37473: 35477,
                37474: 37723,
                37475: 22243,
                37476: 22727,
                37477: 24382,
                37478: 26029,
                37479: 26262,
                37480: 27264,
                37481: 27573,
                37482: 30007,
                37483: 35527,
                37484: 20516,
                37485: 30693,
                37486: 22320,
                37487: 24347,
                37488: 24677,
                37489: 26234,
                37490: 27744,
                37491: 30196,
                37492: 31258,
                37493: 32622,
                37494: 33268,
                37495: 34584,
                37496: 36933,
                37497: 39347,
                37498: 31689,
                37499: 30044,
                37500: 31481,
                37501: 31569,
                37502: 33988,
                37504: 36880,
                37505: 31209,
                37506: 31378,
                37507: 33590,
                37508: 23265,
                37509: 30528,
                37510: 20013,
                37511: 20210,
                37512: 23449,
                37513: 24544,
                37514: 25277,
                37515: 26172,
                37516: 26609,
                37517: 27880,
                37518: 34411,
                37519: 34935,
                37520: 35387,
                37521: 37198,
                37522: 37619,
                37523: 39376,
                37524: 27159,
                37525: 28710,
                37526: 29482,
                37527: 33511,
                37528: 33879,
                37529: 36015,
                37530: 19969,
                37531: 20806,
                37532: 20939,
                37533: 21899,
                37534: 23541,
                37535: 24086,
                37536: 24115,
                37537: 24193,
                37538: 24340,
                37539: 24373,
                37540: 24427,
                37541: 24500,
                37542: 25074,
                37543: 25361,
                37544: 26274,
                37545: 26397,
                37546: 28526,
                37547: 29266,
                37548: 30010,
                37549: 30522,
                37550: 32884,
                37551: 33081,
                37552: 33144,
                37553: 34678,
                37554: 35519,
                37555: 35548,
                37556: 36229,
                37557: 36339,
                37558: 37530,
                37559: 38263,
                37560: 38914,
                37561: 40165,
                37562: 21189,
                37563: 25431,
                37564: 30452,
                37565: 26389,
                37566: 27784,
                37567: 29645,
                37568: 36035,
                37569: 37806,
                37570: 38515,
                37571: 27941,
                37572: 22684,
                37573: 26894,
                37574: 27084,
                37575: 36861,
                37576: 37786,
                37577: 30171,
                37578: 36890,
                37579: 22618,
                37580: 26626,
                37581: 25524,
                37582: 27131,
                37583: 20291,
                37584: 28460,
                37585: 26584,
                37586: 36795,
                37587: 34086,
                37588: 32180,
                37589: 37716,
                37590: 26943,
                37591: 28528,
                37592: 22378,
                37593: 22775,
                37594: 23340,
                37595: 32044,
                37596: 29226,
                37597: 21514,
                37598: 37347,
                37599: 40372,
                37600: 20141,
                37601: 20302,
                37602: 20572,
                37603: 20597,
                37604: 21059,
                37605: 35998,
                37606: 21576,
                37607: 22564,
                37608: 23450,
                37609: 24093,
                37610: 24213,
                37611: 24237,
                37612: 24311,
                37613: 24351,
                37614: 24716,
                37615: 25269,
                37616: 25402,
                37617: 25552,
                37618: 26799,
                37619: 27712,
                37620: 30855,
                37621: 31118,
                37622: 31243,
                37623: 32224,
                37624: 33351,
                37625: 35330,
                37626: 35558,
                37627: 36420,
                37628: 36883,
                37696: 37048,
                37697: 37165,
                37698: 37336,
                37699: 40718,
                37700: 27877,
                37701: 25688,
                37702: 25826,
                37703: 25973,
                37704: 28404,
                37705: 30340,
                37706: 31515,
                37707: 36969,
                37708: 37841,
                37709: 28346,
                37710: 21746,
                37711: 24505,
                37712: 25764,
                37713: 36685,
                37714: 36845,
                37715: 37444,
                37716: 20856,
                37717: 22635,
                37718: 22825,
                37719: 23637,
                37720: 24215,
                37721: 28155,
                37722: 32399,
                37723: 29980,
                37724: 36028,
                37725: 36578,
                37726: 39003,
                37727: 28857,
                37728: 20253,
                37729: 27583,
                37730: 28593,
                37731: 3e4,
                37732: 38651,
                37733: 20814,
                37734: 21520,
                37735: 22581,
                37736: 22615,
                37737: 22956,
                37738: 23648,
                37739: 24466,
                37740: 26007,
                37741: 26460,
                37742: 28193,
                37743: 30331,
                37744: 33759,
                37745: 36077,
                37746: 36884,
                37747: 37117,
                37748: 37709,
                37749: 30757,
                37750: 30778,
                37751: 21162,
                37752: 24230,
                37753: 22303,
                37754: 22900,
                37755: 24594,
                37756: 20498,
                37757: 20826,
                37758: 20908,
                37760: 20941,
                37761: 20992,
                37762: 21776,
                37763: 22612,
                37764: 22616,
                37765: 22871,
                37766: 23445,
                37767: 23798,
                37768: 23947,
                37769: 24764,
                37770: 25237,
                37771: 25645,
                37772: 26481,
                37773: 26691,
                37774: 26812,
                37775: 26847,
                37776: 30423,
                37777: 28120,
                37778: 28271,
                37779: 28059,
                37780: 28783,
                37781: 29128,
                37782: 24403,
                37783: 30168,
                37784: 31095,
                37785: 31561,
                37786: 31572,
                37787: 31570,
                37788: 31958,
                37789: 32113,
                37790: 21040,
                37791: 33891,
                37792: 34153,
                37793: 34276,
                37794: 35342,
                37795: 35588,
                37796: 35910,
                37797: 36367,
                37798: 36867,
                37799: 36879,
                37800: 37913,
                37801: 38518,
                37802: 38957,
                37803: 39472,
                37804: 38360,
                37805: 20685,
                37806: 21205,
                37807: 21516,
                37808: 22530,
                37809: 23566,
                37810: 24999,
                37811: 25758,
                37812: 27934,
                37813: 30643,
                37814: 31461,
                37815: 33012,
                37816: 33796,
                37817: 36947,
                37818: 37509,
                37819: 23776,
                37820: 40199,
                37821: 21311,
                37822: 24471,
                37823: 24499,
                37824: 28060,
                37825: 29305,
                37826: 30563,
                37827: 31167,
                37828: 31716,
                37829: 27602,
                37830: 29420,
                37831: 35501,
                37832: 26627,
                37833: 27233,
                37834: 20984,
                37835: 31361,
                37836: 26932,
                37837: 23626,
                37838: 40182,
                37839: 33515,
                37840: 23493,
                37841: 37193,
                37842: 28702,
                37843: 22136,
                37844: 23663,
                37845: 24775,
                37846: 25958,
                37847: 27788,
                37848: 35930,
                37849: 36929,
                37850: 38931,
                37851: 21585,
                37852: 26311,
                37853: 37389,
                37854: 22856,
                37855: 37027,
                37856: 20869,
                37857: 20045,
                37858: 20970,
                37859: 34201,
                37860: 35598,
                37861: 28760,
                37862: 25466,
                37863: 37707,
                37864: 26978,
                37865: 39348,
                37866: 32260,
                37867: 30071,
                37868: 21335,
                37869: 26976,
                37870: 36575,
                37871: 38627,
                37872: 27741,
                37873: 20108,
                37874: 23612,
                37875: 24336,
                37876: 36841,
                37877: 21250,
                37878: 36049,
                37879: 32905,
                37880: 34425,
                37881: 24319,
                37882: 26085,
                37883: 20083,
                37884: 20837,
                37952: 22914,
                37953: 23615,
                37954: 38894,
                37955: 20219,
                37956: 22922,
                37957: 24525,
                37958: 35469,
                37959: 28641,
                37960: 31152,
                37961: 31074,
                37962: 23527,
                37963: 33905,
                37964: 29483,
                37965: 29105,
                37966: 24180,
                37967: 24565,
                37968: 25467,
                37969: 25754,
                37970: 29123,
                37971: 31896,
                37972: 20035,
                37973: 24316,
                37974: 20043,
                37975: 22492,
                37976: 22178,
                37977: 24745,
                37978: 28611,
                37979: 32013,
                37980: 33021,
                37981: 33075,
                37982: 33215,
                37983: 36786,
                37984: 35223,
                37985: 34468,
                37986: 24052,
                37987: 25226,
                37988: 25773,
                37989: 35207,
                37990: 26487,
                37991: 27874,
                37992: 27966,
                37993: 29750,
                37994: 30772,
                37995: 23110,
                37996: 32629,
                37997: 33453,
                37998: 39340,
                37999: 20467,
                38e3: 24259,
                38001: 25309,
                38002: 25490,
                38003: 25943,
                38004: 26479,
                38005: 30403,
                38006: 29260,
                38007: 32972,
                38008: 32954,
                38009: 36649,
                38010: 37197,
                38011: 20493,
                38012: 22521,
                38013: 23186,
                38014: 26757,
                38016: 26995,
                38017: 29028,
                38018: 29437,
                38019: 36023,
                38020: 22770,
                38021: 36064,
                38022: 38506,
                38023: 36889,
                38024: 34687,
                38025: 31204,
                38026: 30695,
                38027: 33833,
                38028: 20271,
                38029: 21093,
                38030: 21338,
                38031: 25293,
                38032: 26575,
                38033: 27850,
                38034: 30333,
                38035: 31636,
                38036: 31893,
                38037: 33334,
                38038: 34180,
                38039: 36843,
                38040: 26333,
                38041: 28448,
                38042: 29190,
                38043: 32283,
                38044: 33707,
                38045: 39361,
                38046: 40614,
                38047: 20989,
                38048: 31665,
                38049: 30834,
                38050: 31672,
                38051: 32903,
                38052: 31560,
                38053: 27368,
                38054: 24161,
                38055: 32908,
                38056: 30033,
                38057: 30048,
                38058: 20843,
                38059: 37474,
                38060: 28300,
                38061: 30330,
                38062: 37271,
                38063: 39658,
                38064: 20240,
                38065: 32624,
                38066: 25244,
                38067: 31567,
                38068: 38309,
                38069: 40169,
                38070: 22138,
                38071: 22617,
                38072: 34532,
                38073: 38588,
                38074: 20276,
                38075: 21028,
                38076: 21322,
                38077: 21453,
                38078: 21467,
                38079: 24070,
                38080: 25644,
                38081: 26001,
                38082: 26495,
                38083: 27710,
                38084: 27726,
                38085: 29256,
                38086: 29359,
                38087: 29677,
                38088: 30036,
                38089: 32321,
                38090: 33324,
                38091: 34281,
                38092: 36009,
                38093: 31684,
                38094: 37318,
                38095: 29033,
                38096: 38930,
                38097: 39151,
                38098: 25405,
                38099: 26217,
                38100: 30058,
                38101: 30436,
                38102: 30928,
                38103: 34115,
                38104: 34542,
                38105: 21290,
                38106: 21329,
                38107: 21542,
                38108: 22915,
                38109: 24199,
                38110: 24444,
                38111: 24754,
                38112: 25161,
                38113: 25209,
                38114: 25259,
                38115: 26e3,
                38116: 27604,
                38117: 27852,
                38118: 30130,
                38119: 30382,
                38120: 30865,
                38121: 31192,
                38122: 32203,
                38123: 32631,
                38124: 32933,
                38125: 34987,
                38126: 35513,
                38127: 36027,
                38128: 36991,
                38129: 38750,
                38130: 39131,
                38131: 27147,
                38132: 31800,
                38133: 20633,
                38134: 23614,
                38135: 24494,
                38136: 26503,
                38137: 27608,
                38138: 29749,
                38139: 30473,
                38140: 32654,
                38208: 40763,
                38209: 26570,
                38210: 31255,
                38211: 21305,
                38212: 30091,
                38213: 39661,
                38214: 24422,
                38215: 33181,
                38216: 33777,
                38217: 32920,
                38218: 24380,
                38219: 24517,
                38220: 30050,
                38221: 31558,
                38222: 36924,
                38223: 26727,
                38224: 23019,
                38225: 23195,
                38226: 32016,
                38227: 30334,
                38228: 35628,
                38229: 20469,
                38230: 24426,
                38231: 27161,
                38232: 27703,
                38233: 28418,
                38234: 29922,
                38235: 31080,
                38236: 34920,
                38237: 35413,
                38238: 35961,
                38239: 24287,
                38240: 25551,
                38241: 30149,
                38242: 31186,
                38243: 33495,
                38244: 37672,
                38245: 37618,
                38246: 33948,
                38247: 34541,
                38248: 39981,
                38249: 21697,
                38250: 24428,
                38251: 25996,
                38252: 27996,
                38253: 28693,
                38254: 36007,
                38255: 36051,
                38256: 38971,
                38257: 25935,
                38258: 29942,
                38259: 19981,
                38260: 20184,
                38261: 22496,
                38262: 22827,
                38263: 23142,
                38264: 23500,
                38265: 20904,
                38266: 24067,
                38267: 24220,
                38268: 24598,
                38269: 25206,
                38270: 25975,
                38272: 26023,
                38273: 26222,
                38274: 28014,
                38275: 29238,
                38276: 31526,
                38277: 33104,
                38278: 33178,
                38279: 33433,
                38280: 35676,
                38281: 36e3,
                38282: 36070,
                38283: 36212,
                38284: 38428,
                38285: 38468,
                38286: 20398,
                38287: 25771,
                38288: 27494,
                38289: 33310,
                38290: 33889,
                38291: 34154,
                38292: 37096,
                38293: 23553,
                38294: 26963,
                38295: 39080,
                38296: 33914,
                38297: 34135,
                38298: 20239,
                38299: 21103,
                38300: 24489,
                38301: 24133,
                38302: 26381,
                38303: 31119,
                38304: 33145,
                38305: 35079,
                38306: 35206,
                38307: 28149,
                38308: 24343,
                38309: 25173,
                38310: 27832,
                38311: 20175,
                38312: 29289,
                38313: 39826,
                38314: 20998,
                38315: 21563,
                38316: 22132,
                38317: 22707,
                38318: 24996,
                38319: 25198,
                38320: 28954,
                38321: 22894,
                38322: 31881,
                38323: 31966,
                38324: 32027,
                38325: 38640,
                38326: 25991,
                38327: 32862,
                38328: 19993,
                38329: 20341,
                38330: 20853,
                38331: 22592,
                38332: 24163,
                38333: 24179,
                38334: 24330,
                38335: 26564,
                38336: 20006,
                38337: 34109,
                38338: 38281,
                38339: 38491,
                38340: 31859,
                38341: 38913,
                38342: 20731,
                38343: 22721,
                38344: 30294,
                38345: 30887,
                38346: 21029,
                38347: 30629,
                38348: 34065,
                38349: 31622,
                38350: 20559,
                38351: 22793,
                38352: 29255,
                38353: 31687,
                38354: 32232,
                38355: 36794,
                38356: 36820,
                38357: 36941,
                38358: 20415,
                38359: 21193,
                38360: 23081,
                38361: 24321,
                38362: 38829,
                38363: 20445,
                38364: 33303,
                38365: 37610,
                38366: 22275,
                38367: 25429,
                38368: 27497,
                38369: 29995,
                38370: 35036,
                38371: 36628,
                38372: 31298,
                38373: 21215,
                38374: 22675,
                38375: 24917,
                38376: 25098,
                38377: 26286,
                38378: 27597,
                38379: 31807,
                38380: 33769,
                38381: 20515,
                38382: 20472,
                38383: 21253,
                38384: 21574,
                38385: 22577,
                38386: 22857,
                38387: 23453,
                38388: 23792,
                38389: 23791,
                38390: 23849,
                38391: 24214,
                38392: 25265,
                38393: 25447,
                38394: 25918,
                38395: 26041,
                38396: 26379,
                38464: 27861,
                38465: 27873,
                38466: 28921,
                38467: 30770,
                38468: 32299,
                38469: 32990,
                38470: 33459,
                38471: 33804,
                38472: 34028,
                38473: 34562,
                38474: 35090,
                38475: 35370,
                38476: 35914,
                38477: 37030,
                38478: 37586,
                38479: 39165,
                38480: 40179,
                38481: 40300,
                38482: 20047,
                38483: 20129,
                38484: 20621,
                38485: 21078,
                38486: 22346,
                38487: 22952,
                38488: 24125,
                38489: 24536,
                38490: 24537,
                38491: 25151,
                38492: 26292,
                38493: 26395,
                38494: 26576,
                38495: 26834,
                38496: 20882,
                38497: 32033,
                38498: 32938,
                38499: 33192,
                38500: 35584,
                38501: 35980,
                38502: 36031,
                38503: 37502,
                38504: 38450,
                38505: 21536,
                38506: 38956,
                38507: 21271,
                38508: 20693,
                38509: 21340,
                38510: 22696,
                38511: 25778,
                38512: 26420,
                38513: 29287,
                38514: 30566,
                38515: 31302,
                38516: 37350,
                38517: 21187,
                38518: 27809,
                38519: 27526,
                38520: 22528,
                38521: 24140,
                38522: 22868,
                38523: 26412,
                38524: 32763,
                38525: 20961,
                38526: 30406,
                38528: 25705,
                38529: 30952,
                38530: 39764,
                38531: 40635,
                38532: 22475,
                38533: 22969,
                38534: 26151,
                38535: 26522,
                38536: 27598,
                38537: 21737,
                38538: 27097,
                38539: 24149,
                38540: 33180,
                38541: 26517,
                38542: 39850,
                38543: 26622,
                38544: 40018,
                38545: 26717,
                38546: 20134,
                38547: 20451,
                38548: 21448,
                38549: 25273,
                38550: 26411,
                38551: 27819,
                38552: 36804,
                38553: 20397,
                38554: 32365,
                38555: 40639,
                38556: 19975,
                38557: 24930,
                38558: 28288,
                38559: 28459,
                38560: 34067,
                38561: 21619,
                38562: 26410,
                38563: 39749,
                38564: 24051,
                38565: 31637,
                38566: 23724,
                38567: 23494,
                38568: 34588,
                38569: 28234,
                38570: 34001,
                38571: 31252,
                38572: 33032,
                38573: 22937,
                38574: 31885,
                38575: 27665,
                38576: 30496,
                38577: 21209,
                38578: 22818,
                38579: 28961,
                38580: 29279,
                38581: 30683,
                38582: 38695,
                38583: 40289,
                38584: 26891,
                38585: 23167,
                38586: 23064,
                38587: 20901,
                38588: 21517,
                38589: 21629,
                38590: 26126,
                38591: 30431,
                38592: 36855,
                38593: 37528,
                38594: 40180,
                38595: 23018,
                38596: 29277,
                38597: 28357,
                38598: 20813,
                38599: 26825,
                38600: 32191,
                38601: 32236,
                38602: 38754,
                38603: 40634,
                38604: 25720,
                38605: 27169,
                38606: 33538,
                38607: 22916,
                38608: 23391,
                38609: 27611,
                38610: 29467,
                38611: 30450,
                38612: 32178,
                38613: 32791,
                38614: 33945,
                38615: 20786,
                38616: 26408,
                38617: 40665,
                38618: 30446,
                38619: 26466,
                38620: 21247,
                38621: 39173,
                38622: 23588,
                38623: 25147,
                38624: 31870,
                38625: 36016,
                38626: 21839,
                38627: 24758,
                38628: 32011,
                38629: 38272,
                38630: 21249,
                38631: 20063,
                38632: 20918,
                38633: 22812,
                38634: 29242,
                38635: 32822,
                38636: 37326,
                38637: 24357,
                38638: 30690,
                38639: 21380,
                38640: 24441,
                38641: 32004,
                38642: 34220,
                38643: 35379,
                38644: 36493,
                38645: 38742,
                38646: 26611,
                38647: 34222,
                38648: 37971,
                38649: 24841,
                38650: 24840,
                38651: 27833,
                38652: 30290,
                38720: 35565,
                38721: 36664,
                38722: 21807,
                38723: 20305,
                38724: 20778,
                38725: 21191,
                38726: 21451,
                38727: 23461,
                38728: 24189,
                38729: 24736,
                38730: 24962,
                38731: 25558,
                38732: 26377,
                38733: 26586,
                38734: 28263,
                38735: 28044,
                38736: 29494,
                38737: 29495,
                38738: 30001,
                38739: 31056,
                38740: 35029,
                38741: 35480,
                38742: 36938,
                38743: 37009,
                38744: 37109,
                38745: 38596,
                38746: 34701,
                38747: 22805,
                38748: 20104,
                38749: 20313,
                38750: 19982,
                38751: 35465,
                38752: 36671,
                38753: 38928,
                38754: 20653,
                38755: 24188,
                38756: 22934,
                38757: 23481,
                38758: 24248,
                38759: 25562,
                38760: 25594,
                38761: 25793,
                38762: 26332,
                38763: 26954,
                38764: 27096,
                38765: 27915,
                38766: 28342,
                38767: 29076,
                38768: 29992,
                38769: 31407,
                38770: 32650,
                38771: 32768,
                38772: 33865,
                38773: 33993,
                38774: 35201,
                38775: 35617,
                38776: 36362,
                38777: 36965,
                38778: 38525,
                38779: 39178,
                38780: 24958,
                38781: 25233,
                38782: 27442,
                38784: 27779,
                38785: 28020,
                38786: 32716,
                38787: 32764,
                38788: 28096,
                38789: 32645,
                38790: 34746,
                38791: 35064,
                38792: 26469,
                38793: 33713,
                38794: 38972,
                38795: 38647,
                38796: 27931,
                38797: 32097,
                38798: 33853,
                38799: 37226,
                38800: 20081,
                38801: 21365,
                38802: 23888,
                38803: 27396,
                38804: 28651,
                38805: 34253,
                38806: 34349,
                38807: 35239,
                38808: 21033,
                38809: 21519,
                38810: 23653,
                38811: 26446,
                38812: 26792,
                38813: 29702,
                38814: 29827,
                38815: 30178,
                38816: 35023,
                38817: 35041,
                38818: 37324,
                38819: 38626,
                38820: 38520,
                38821: 24459,
                38822: 29575,
                38823: 31435,
                38824: 33870,
                38825: 25504,
                38826: 30053,
                38827: 21129,
                38828: 27969,
                38829: 28316,
                38830: 29705,
                38831: 30041,
                38832: 30827,
                38833: 31890,
                38834: 38534,
                38835: 31452,
                38836: 40845,
                38837: 20406,
                38838: 24942,
                38839: 26053,
                38840: 34396,
                38841: 20102,
                38842: 20142,
                38843: 20698,
                38844: 20001,
                38845: 20940,
                38846: 23534,
                38847: 26009,
                38848: 26753,
                38849: 28092,
                38850: 29471,
                38851: 30274,
                38852: 30637,
                38853: 31260,
                38854: 31975,
                38855: 33391,
                38856: 35538,
                38857: 36988,
                38858: 37327,
                38859: 38517,
                38860: 38936,
                38861: 21147,
                38862: 32209,
                38863: 20523,
                38864: 21400,
                38865: 26519,
                38866: 28107,
                38867: 29136,
                38868: 29747,
                38869: 33256,
                38870: 36650,
                38871: 38563,
                38872: 40023,
                38873: 40607,
                38874: 29792,
                38875: 22593,
                38876: 28057,
                38877: 32047,
                38878: 39006,
                38879: 20196,
                38880: 20278,
                38881: 20363,
                38882: 20919,
                38883: 21169,
                38884: 23994,
                38885: 24604,
                38886: 29618,
                38887: 31036,
                38888: 33491,
                38889: 37428,
                38890: 38583,
                38891: 38646,
                38892: 38666,
                38893: 40599,
                38894: 40802,
                38895: 26278,
                38896: 27508,
                38897: 21015,
                38898: 21155,
                38899: 28872,
                38900: 35010,
                38901: 24265,
                38902: 24651,
                38903: 24976,
                38904: 28451,
                38905: 29001,
                38906: 31806,
                38907: 32244,
                38908: 32879,
                38976: 34030,
                38977: 36899,
                38978: 37676,
                38979: 21570,
                38980: 39791,
                38981: 27347,
                38982: 28809,
                38983: 36034,
                38984: 36335,
                38985: 38706,
                38986: 21172,
                38987: 23105,
                38988: 24266,
                38989: 24324,
                38990: 26391,
                38991: 27004,
                38992: 27028,
                38993: 28010,
                38994: 28431,
                38995: 29282,
                38996: 29436,
                38997: 31725,
                38998: 32769,
                38999: 32894,
                39e3: 34635,
                39001: 37070,
                39002: 20845,
                39003: 40595,
                39004: 31108,
                39005: 32907,
                39006: 37682,
                39007: 35542,
                39008: 20525,
                39009: 21644,
                39010: 35441,
                39011: 27498,
                39012: 36036,
                39013: 33031,
                39014: 24785,
                39015: 26528,
                39016: 40434,
                39017: 20121,
                39018: 20120,
                39019: 39952,
                39020: 35435,
                39021: 34241,
                39022: 34152,
                39023: 26880,
                39024: 28286,
                39025: 30871,
                39026: 33109,
                39071: 24332,
                39072: 19984,
                39073: 19989,
                39074: 20010,
                39075: 20017,
                39076: 20022,
                39077: 20028,
                39078: 20031,
                39079: 20034,
                39080: 20054,
                39081: 20056,
                39082: 20098,
                39083: 20101,
                39084: 35947,
                39085: 20106,
                39086: 33298,
                39087: 24333,
                39088: 20110,
                39089: 20126,
                39090: 20127,
                39091: 20128,
                39092: 20130,
                39093: 20144,
                39094: 20147,
                39095: 20150,
                39096: 20174,
                39097: 20173,
                39098: 20164,
                39099: 20166,
                39100: 20162,
                39101: 20183,
                39102: 20190,
                39103: 20205,
                39104: 20191,
                39105: 20215,
                39106: 20233,
                39107: 20314,
                39108: 20272,
                39109: 20315,
                39110: 20317,
                39111: 20311,
                39112: 20295,
                39113: 20342,
                39114: 20360,
                39115: 20367,
                39116: 20376,
                39117: 20347,
                39118: 20329,
                39119: 20336,
                39120: 20369,
                39121: 20335,
                39122: 20358,
                39123: 20374,
                39124: 20760,
                39125: 20436,
                39126: 20447,
                39127: 20430,
                39128: 20440,
                39129: 20443,
                39130: 20433,
                39131: 20442,
                39132: 20432,
                39133: 20452,
                39134: 20453,
                39135: 20506,
                39136: 20520,
                39137: 20500,
                39138: 20522,
                39139: 20517,
                39140: 20485,
                39141: 20252,
                39142: 20470,
                39143: 20513,
                39144: 20521,
                39145: 20524,
                39146: 20478,
                39147: 20463,
                39148: 20497,
                39149: 20486,
                39150: 20547,
                39151: 20551,
                39152: 26371,
                39153: 20565,
                39154: 20560,
                39155: 20552,
                39156: 20570,
                39157: 20566,
                39158: 20588,
                39159: 20600,
                39160: 20608,
                39161: 20634,
                39162: 20613,
                39163: 20660,
                39164: 20658,
                39232: 20681,
                39233: 20682,
                39234: 20659,
                39235: 20674,
                39236: 20694,
                39237: 20702,
                39238: 20709,
                39239: 20717,
                39240: 20707,
                39241: 20718,
                39242: 20729,
                39243: 20725,
                39244: 20745,
                39245: 20737,
                39246: 20738,
                39247: 20758,
                39248: 20757,
                39249: 20756,
                39250: 20762,
                39251: 20769,
                39252: 20794,
                39253: 20791,
                39254: 20796,
                39255: 20795,
                39256: 20799,
                39257: 20800,
                39258: 20818,
                39259: 20812,
                39260: 20820,
                39261: 20834,
                39262: 31480,
                39263: 20841,
                39264: 20842,
                39265: 20846,
                39266: 20864,
                39267: 20866,
                39268: 22232,
                39269: 20876,
                39270: 20873,
                39271: 20879,
                39272: 20881,
                39273: 20883,
                39274: 20885,
                39275: 20886,
                39276: 20900,
                39277: 20902,
                39278: 20898,
                39279: 20905,
                39280: 20906,
                39281: 20907,
                39282: 20915,
                39283: 20913,
                39284: 20914,
                39285: 20912,
                39286: 20917,
                39287: 20925,
                39288: 20933,
                39289: 20937,
                39290: 20955,
                39291: 20960,
                39292: 34389,
                39293: 20969,
                39294: 20973,
                39296: 20976,
                39297: 20981,
                39298: 20990,
                39299: 20996,
                39300: 21003,
                39301: 21012,
                39302: 21006,
                39303: 21031,
                39304: 21034,
                39305: 21038,
                39306: 21043,
                39307: 21049,
                39308: 21071,
                39309: 21060,
                39310: 21067,
                39311: 21068,
                39312: 21086,
                39313: 21076,
                39314: 21098,
                39315: 21108,
                39316: 21097,
                39317: 21107,
                39318: 21119,
                39319: 21117,
                39320: 21133,
                39321: 21140,
                39322: 21138,
                39323: 21105,
                39324: 21128,
                39325: 21137,
                39326: 36776,
                39327: 36775,
                39328: 21164,
                39329: 21165,
                39330: 21180,
                39331: 21173,
                39332: 21185,
                39333: 21197,
                39334: 21207,
                39335: 21214,
                39336: 21219,
                39337: 21222,
                39338: 39149,
                39339: 21216,
                39340: 21235,
                39341: 21237,
                39342: 21240,
                39343: 21241,
                39344: 21254,
                39345: 21256,
                39346: 30008,
                39347: 21261,
                39348: 21264,
                39349: 21263,
                39350: 21269,
                39351: 21274,
                39352: 21283,
                39353: 21295,
                39354: 21297,
                39355: 21299,
                39356: 21304,
                39357: 21312,
                39358: 21318,
                39359: 21317,
                39360: 19991,
                39361: 21321,
                39362: 21325,
                39363: 20950,
                39364: 21342,
                39365: 21353,
                39366: 21358,
                39367: 22808,
                39368: 21371,
                39369: 21367,
                39370: 21378,
                39371: 21398,
                39372: 21408,
                39373: 21414,
                39374: 21413,
                39375: 21422,
                39376: 21424,
                39377: 21430,
                39378: 21443,
                39379: 31762,
                39380: 38617,
                39381: 21471,
                39382: 26364,
                39383: 29166,
                39384: 21486,
                39385: 21480,
                39386: 21485,
                39387: 21498,
                39388: 21505,
                39389: 21565,
                39390: 21568,
                39391: 21548,
                39392: 21549,
                39393: 21564,
                39394: 21550,
                39395: 21558,
                39396: 21545,
                39397: 21533,
                39398: 21582,
                39399: 21647,
                39400: 21621,
                39401: 21646,
                39402: 21599,
                39403: 21617,
                39404: 21623,
                39405: 21616,
                39406: 21650,
                39407: 21627,
                39408: 21632,
                39409: 21622,
                39410: 21636,
                39411: 21648,
                39412: 21638,
                39413: 21703,
                39414: 21666,
                39415: 21688,
                39416: 21669,
                39417: 21676,
                39418: 21700,
                39419: 21704,
                39420: 21672,
                39488: 21675,
                39489: 21698,
                39490: 21668,
                39491: 21694,
                39492: 21692,
                39493: 21720,
                39494: 21733,
                39495: 21734,
                39496: 21775,
                39497: 21780,
                39498: 21757,
                39499: 21742,
                39500: 21741,
                39501: 21754,
                39502: 21730,
                39503: 21817,
                39504: 21824,
                39505: 21859,
                39506: 21836,
                39507: 21806,
                39508: 21852,
                39509: 21829,
                39510: 21846,
                39511: 21847,
                39512: 21816,
                39513: 21811,
                39514: 21853,
                39515: 21913,
                39516: 21888,
                39517: 21679,
                39518: 21898,
                39519: 21919,
                39520: 21883,
                39521: 21886,
                39522: 21912,
                39523: 21918,
                39524: 21934,
                39525: 21884,
                39526: 21891,
                39527: 21929,
                39528: 21895,
                39529: 21928,
                39530: 21978,
                39531: 21957,
                39532: 21983,
                39533: 21956,
                39534: 21980,
                39535: 21988,
                39536: 21972,
                39537: 22036,
                39538: 22007,
                39539: 22038,
                39540: 22014,
                39541: 22013,
                39542: 22043,
                39543: 22009,
                39544: 22094,
                39545: 22096,
                39546: 29151,
                39547: 22068,
                39548: 22070,
                39549: 22066,
                39550: 22072,
                39552: 22123,
                39553: 22116,
                39554: 22063,
                39555: 22124,
                39556: 22122,
                39557: 22150,
                39558: 22144,
                39559: 22154,
                39560: 22176,
                39561: 22164,
                39562: 22159,
                39563: 22181,
                39564: 22190,
                39565: 22198,
                39566: 22196,
                39567: 22210,
                39568: 22204,
                39569: 22209,
                39570: 22211,
                39571: 22208,
                39572: 22216,
                39573: 22222,
                39574: 22225,
                39575: 22227,
                39576: 22231,
                39577: 22254,
                39578: 22265,
                39579: 22272,
                39580: 22271,
                39581: 22276,
                39582: 22281,
                39583: 22280,
                39584: 22283,
                39585: 22285,
                39586: 22291,
                39587: 22296,
                39588: 22294,
                39589: 21959,
                39590: 22300,
                39591: 22310,
                39592: 22327,
                39593: 22328,
                39594: 22350,
                39595: 22331,
                39596: 22336,
                39597: 22351,
                39598: 22377,
                39599: 22464,
                39600: 22408,
                39601: 22369,
                39602: 22399,
                39603: 22409,
                39604: 22419,
                39605: 22432,
                39606: 22451,
                39607: 22436,
                39608: 22442,
                39609: 22448,
                39610: 22467,
                39611: 22470,
                39612: 22484,
                39613: 22482,
                39614: 22483,
                39615: 22538,
                39616: 22486,
                39617: 22499,
                39618: 22539,
                39619: 22553,
                39620: 22557,
                39621: 22642,
                39622: 22561,
                39623: 22626,
                39624: 22603,
                39625: 22640,
                39626: 27584,
                39627: 22610,
                39628: 22589,
                39629: 22649,
                39630: 22661,
                39631: 22713,
                39632: 22687,
                39633: 22699,
                39634: 22714,
                39635: 22750,
                39636: 22715,
                39637: 22712,
                39638: 22702,
                39639: 22725,
                39640: 22739,
                39641: 22737,
                39642: 22743,
                39643: 22745,
                39644: 22744,
                39645: 22757,
                39646: 22748,
                39647: 22756,
                39648: 22751,
                39649: 22767,
                39650: 22778,
                39651: 22777,
                39652: 22779,
                39653: 22780,
                39654: 22781,
                39655: 22786,
                39656: 22794,
                39657: 22800,
                39658: 22811,
                39659: 26790,
                39660: 22821,
                39661: 22828,
                39662: 22829,
                39663: 22834,
                39664: 22840,
                39665: 22846,
                39666: 31442,
                39667: 22869,
                39668: 22864,
                39669: 22862,
                39670: 22874,
                39671: 22872,
                39672: 22882,
                39673: 22880,
                39674: 22887,
                39675: 22892,
                39676: 22889,
                39744: 22904,
                39745: 22913,
                39746: 22941,
                39747: 20318,
                39748: 20395,
                39749: 22947,
                39750: 22962,
                39751: 22982,
                39752: 23016,
                39753: 23004,
                39754: 22925,
                39755: 23001,
                39756: 23002,
                39757: 23077,
                39758: 23071,
                39759: 23057,
                39760: 23068,
                39761: 23049,
                39762: 23066,
                39763: 23104,
                39764: 23148,
                39765: 23113,
                39766: 23093,
                39767: 23094,
                39768: 23138,
                39769: 23146,
                39770: 23194,
                39771: 23228,
                39772: 23230,
                39773: 23243,
                39774: 23234,
                39775: 23229,
                39776: 23267,
                39777: 23255,
                39778: 23270,
                39779: 23273,
                39780: 23254,
                39781: 23290,
                39782: 23291,
                39783: 23308,
                39784: 23307,
                39785: 23318,
                39786: 23346,
                39787: 23248,
                39788: 23338,
                39789: 23350,
                39790: 23358,
                39791: 23363,
                39792: 23365,
                39793: 23360,
                39794: 23377,
                39795: 23381,
                39796: 23386,
                39797: 23387,
                39798: 23397,
                39799: 23401,
                39800: 23408,
                39801: 23411,
                39802: 23413,
                39803: 23416,
                39804: 25992,
                39805: 23418,
                39806: 23424,
                39808: 23427,
                39809: 23462,
                39810: 23480,
                39811: 23491,
                39812: 23495,
                39813: 23497,
                39814: 23508,
                39815: 23504,
                39816: 23524,
                39817: 23526,
                39818: 23522,
                39819: 23518,
                39820: 23525,
                39821: 23531,
                39822: 23536,
                39823: 23542,
                39824: 23539,
                39825: 23557,
                39826: 23559,
                39827: 23560,
                39828: 23565,
                39829: 23571,
                39830: 23584,
                39831: 23586,
                39832: 23592,
                39833: 23608,
                39834: 23609,
                39835: 23617,
                39836: 23622,
                39837: 23630,
                39838: 23635,
                39839: 23632,
                39840: 23631,
                39841: 23409,
                39842: 23660,
                39843: 23662,
                39844: 20066,
                39845: 23670,
                39846: 23673,
                39847: 23692,
                39848: 23697,
                39849: 23700,
                39850: 22939,
                39851: 23723,
                39852: 23739,
                39853: 23734,
                39854: 23740,
                39855: 23735,
                39856: 23749,
                39857: 23742,
                39858: 23751,
                39859: 23769,
                39860: 23785,
                39861: 23805,
                39862: 23802,
                39863: 23789,
                39864: 23948,
                39865: 23786,
                39866: 23819,
                39867: 23829,
                39868: 23831,
                39869: 23900,
                39870: 23839,
                39871: 23835,
                39872: 23825,
                39873: 23828,
                39874: 23842,
                39875: 23834,
                39876: 23833,
                39877: 23832,
                39878: 23884,
                39879: 23890,
                39880: 23886,
                39881: 23883,
                39882: 23916,
                39883: 23923,
                39884: 23926,
                39885: 23943,
                39886: 23940,
                39887: 23938,
                39888: 23970,
                39889: 23965,
                39890: 23980,
                39891: 23982,
                39892: 23997,
                39893: 23952,
                39894: 23991,
                39895: 23996,
                39896: 24009,
                39897: 24013,
                39898: 24019,
                39899: 24018,
                39900: 24022,
                39901: 24027,
                39902: 24043,
                39903: 24050,
                39904: 24053,
                39905: 24075,
                39906: 24090,
                39907: 24089,
                39908: 24081,
                39909: 24091,
                39910: 24118,
                39911: 24119,
                39912: 24132,
                39913: 24131,
                39914: 24128,
                39915: 24142,
                39916: 24151,
                39917: 24148,
                39918: 24159,
                39919: 24162,
                39920: 24164,
                39921: 24135,
                39922: 24181,
                39923: 24182,
                39924: 24186,
                39925: 40636,
                39926: 24191,
                39927: 24224,
                39928: 24257,
                39929: 24258,
                39930: 24264,
                39931: 24272,
                39932: 24271,
                4e4: 24278,
                40001: 24291,
                40002: 24285,
                40003: 24282,
                40004: 24283,
                40005: 24290,
                40006: 24289,
                40007: 24296,
                40008: 24297,
                40009: 24300,
                40010: 24305,
                40011: 24307,
                40012: 24304,
                40013: 24308,
                40014: 24312,
                40015: 24318,
                40016: 24323,
                40017: 24329,
                40018: 24413,
                40019: 24412,
                40020: 24331,
                40021: 24337,
                40022: 24342,
                40023: 24361,
                40024: 24365,
                40025: 24376,
                40026: 24385,
                40027: 24392,
                40028: 24396,
                40029: 24398,
                40030: 24367,
                40031: 24401,
                40032: 24406,
                40033: 24407,
                40034: 24409,
                40035: 24417,
                40036: 24429,
                40037: 24435,
                40038: 24439,
                40039: 24451,
                40040: 24450,
                40041: 24447,
                40042: 24458,
                40043: 24456,
                40044: 24465,
                40045: 24455,
                40046: 24478,
                40047: 24473,
                40048: 24472,
                40049: 24480,
                40050: 24488,
                40051: 24493,
                40052: 24508,
                40053: 24534,
                40054: 24571,
                40055: 24548,
                40056: 24568,
                40057: 24561,
                40058: 24541,
                40059: 24755,
                40060: 24575,
                40061: 24609,
                40062: 24672,
                40064: 24601,
                40065: 24592,
                40066: 24617,
                40067: 24590,
                40068: 24625,
                40069: 24603,
                40070: 24597,
                40071: 24619,
                40072: 24614,
                40073: 24591,
                40074: 24634,
                40075: 24666,
                40076: 24641,
                40077: 24682,
                40078: 24695,
                40079: 24671,
                40080: 24650,
                40081: 24646,
                40082: 24653,
                40083: 24675,
                40084: 24643,
                40085: 24676,
                40086: 24642,
                40087: 24684,
                40088: 24683,
                40089: 24665,
                40090: 24705,
                40091: 24717,
                40092: 24807,
                40093: 24707,
                40094: 24730,
                40095: 24708,
                40096: 24731,
                40097: 24726,
                40098: 24727,
                40099: 24722,
                40100: 24743,
                40101: 24715,
                40102: 24801,
                40103: 24760,
                40104: 24800,
                40105: 24787,
                40106: 24756,
                40107: 24560,
                40108: 24765,
                40109: 24774,
                40110: 24757,
                40111: 24792,
                40112: 24909,
                40113: 24853,
                40114: 24838,
                40115: 24822,
                40116: 24823,
                40117: 24832,
                40118: 24820,
                40119: 24826,
                40120: 24835,
                40121: 24865,
                40122: 24827,
                40123: 24817,
                40124: 24845,
                40125: 24846,
                40126: 24903,
                40127: 24894,
                40128: 24872,
                40129: 24871,
                40130: 24906,
                40131: 24895,
                40132: 24892,
                40133: 24876,
                40134: 24884,
                40135: 24893,
                40136: 24898,
                40137: 24900,
                40138: 24947,
                40139: 24951,
                40140: 24920,
                40141: 24921,
                40142: 24922,
                40143: 24939,
                40144: 24948,
                40145: 24943,
                40146: 24933,
                40147: 24945,
                40148: 24927,
                40149: 24925,
                40150: 24915,
                40151: 24949,
                40152: 24985,
                40153: 24982,
                40154: 24967,
                40155: 25004,
                40156: 24980,
                40157: 24986,
                40158: 24970,
                40159: 24977,
                40160: 25003,
                40161: 25006,
                40162: 25036,
                40163: 25034,
                40164: 25033,
                40165: 25079,
                40166: 25032,
                40167: 25027,
                40168: 25030,
                40169: 25018,
                40170: 25035,
                40171: 32633,
                40172: 25037,
                40173: 25062,
                40174: 25059,
                40175: 25078,
                40176: 25082,
                40177: 25076,
                40178: 25087,
                40179: 25085,
                40180: 25084,
                40181: 25086,
                40182: 25088,
                40183: 25096,
                40184: 25097,
                40185: 25101,
                40186: 25100,
                40187: 25108,
                40188: 25115,
                40256: 25118,
                40257: 25121,
                40258: 25130,
                40259: 25134,
                40260: 25136,
                40261: 25138,
                40262: 25139,
                40263: 25153,
                40264: 25166,
                40265: 25182,
                40266: 25187,
                40267: 25179,
                40268: 25184,
                40269: 25192,
                40270: 25212,
                40271: 25218,
                40272: 25225,
                40273: 25214,
                40274: 25234,
                40275: 25235,
                40276: 25238,
                40277: 25300,
                40278: 25219,
                40279: 25236,
                40280: 25303,
                40281: 25297,
                40282: 25275,
                40283: 25295,
                40284: 25343,
                40285: 25286,
                40286: 25812,
                40287: 25288,
                40288: 25308,
                40289: 25292,
                40290: 25290,
                40291: 25282,
                40292: 25287,
                40293: 25243,
                40294: 25289,
                40295: 25356,
                40296: 25326,
                40297: 25329,
                40298: 25383,
                40299: 25346,
                40300: 25352,
                40301: 25327,
                40302: 25333,
                40303: 25424,
                40304: 25406,
                40305: 25421,
                40306: 25628,
                40307: 25423,
                40308: 25494,
                40309: 25486,
                40310: 25472,
                40311: 25515,
                40312: 25462,
                40313: 25507,
                40314: 25487,
                40315: 25481,
                40316: 25503,
                40317: 25525,
                40318: 25451,
                40320: 25449,
                40321: 25534,
                40322: 25577,
                40323: 25536,
                40324: 25542,
                40325: 25571,
                40326: 25545,
                40327: 25554,
                40328: 25590,
                40329: 25540,
                40330: 25622,
                40331: 25652,
                40332: 25606,
                40333: 25619,
                40334: 25638,
                40335: 25654,
                40336: 25885,
                40337: 25623,
                40338: 25640,
                40339: 25615,
                40340: 25703,
                40341: 25711,
                40342: 25718,
                40343: 25678,
                40344: 25898,
                40345: 25749,
                40346: 25747,
                40347: 25765,
                40348: 25769,
                40349: 25736,
                40350: 25788,
                40351: 25818,
                40352: 25810,
                40353: 25797,
                40354: 25799,
                40355: 25787,
                40356: 25816,
                40357: 25794,
                40358: 25841,
                40359: 25831,
                40360: 33289,
                40361: 25824,
                40362: 25825,
                40363: 25260,
                40364: 25827,
                40365: 25839,
                40366: 25900,
                40367: 25846,
                40368: 25844,
                40369: 25842,
                40370: 25850,
                40371: 25856,
                40372: 25853,
                40373: 25880,
                40374: 25884,
                40375: 25861,
                40376: 25892,
                40377: 25891,
                40378: 25899,
                40379: 25908,
                40380: 25909,
                40381: 25911,
                40382: 25910,
                40383: 25912,
                40384: 30027,
                40385: 25928,
                40386: 25942,
                40387: 25941,
                40388: 25933,
                40389: 25944,
                40390: 25950,
                40391: 25949,
                40392: 25970,
                40393: 25976,
                40394: 25986,
                40395: 25987,
                40396: 35722,
                40397: 26011,
                40398: 26015,
                40399: 26027,
                40400: 26039,
                40401: 26051,
                40402: 26054,
                40403: 26049,
                40404: 26052,
                40405: 26060,
                40406: 26066,
                40407: 26075,
                40408: 26073,
                40409: 26080,
                40410: 26081,
                40411: 26097,
                40412: 26482,
                40413: 26122,
                40414: 26115,
                40415: 26107,
                40416: 26483,
                40417: 26165,
                40418: 26166,
                40419: 26164,
                40420: 26140,
                40421: 26191,
                40422: 26180,
                40423: 26185,
                40424: 26177,
                40425: 26206,
                40426: 26205,
                40427: 26212,
                40428: 26215,
                40429: 26216,
                40430: 26207,
                40431: 26210,
                40432: 26224,
                40433: 26243,
                40434: 26248,
                40435: 26254,
                40436: 26249,
                40437: 26244,
                40438: 26264,
                40439: 26269,
                40440: 26305,
                40441: 26297,
                40442: 26313,
                40443: 26302,
                40444: 26300,
                40512: 26308,
                40513: 26296,
                40514: 26326,
                40515: 26330,
                40516: 26336,
                40517: 26175,
                40518: 26342,
                40519: 26345,
                40520: 26352,
                40521: 26357,
                40522: 26359,
                40523: 26383,
                40524: 26390,
                40525: 26398,
                40526: 26406,
                40527: 26407,
                40528: 38712,
                40529: 26414,
                40530: 26431,
                40531: 26422,
                40532: 26433,
                40533: 26424,
                40534: 26423,
                40535: 26438,
                40536: 26462,
                40537: 26464,
                40538: 26457,
                40539: 26467,
                40540: 26468,
                40541: 26505,
                40542: 26480,
                40543: 26537,
                40544: 26492,
                40545: 26474,
                40546: 26508,
                40547: 26507,
                40548: 26534,
                40549: 26529,
                40550: 26501,
                40551: 26551,
                40552: 26607,
                40553: 26548,
                40554: 26604,
                40555: 26547,
                40556: 26601,
                40557: 26552,
                40558: 26596,
                40559: 26590,
                40560: 26589,
                40561: 26594,
                40562: 26606,
                40563: 26553,
                40564: 26574,
                40565: 26566,
                40566: 26599,
                40567: 27292,
                40568: 26654,
                40569: 26694,
                40570: 26665,
                40571: 26688,
                40572: 26701,
                40573: 26674,
                40574: 26702,
                40576: 26803,
                40577: 26667,
                40578: 26713,
                40579: 26723,
                40580: 26743,
                40581: 26751,
                40582: 26783,
                40583: 26767,
                40584: 26797,
                40585: 26772,
                40586: 26781,
                40587: 26779,
                40588: 26755,
                40589: 27310,
                40590: 26809,
                40591: 26740,
                40592: 26805,
                40593: 26784,
                40594: 26810,
                40595: 26895,
                40596: 26765,
                40597: 26750,
                40598: 26881,
                40599: 26826,
                40600: 26888,
                40601: 26840,
                40602: 26914,
                40603: 26918,
                40604: 26849,
                40605: 26892,
                40606: 26829,
                40607: 26836,
                40608: 26855,
                40609: 26837,
                40610: 26934,
                40611: 26898,
                40612: 26884,
                40613: 26839,
                40614: 26851,
                40615: 26917,
                40616: 26873,
                40617: 26848,
                40618: 26863,
                40619: 26920,
                40620: 26922,
                40621: 26906,
                40622: 26915,
                40623: 26913,
                40624: 26822,
                40625: 27001,
                40626: 26999,
                40627: 26972,
                40628: 27e3,
                40629: 26987,
                40630: 26964,
                40631: 27006,
                40632: 26990,
                40633: 26937,
                40634: 26996,
                40635: 26941,
                40636: 26969,
                40637: 26928,
                40638: 26977,
                40639: 26974,
                40640: 26973,
                40641: 27009,
                40642: 26986,
                40643: 27058,
                40644: 27054,
                40645: 27088,
                40646: 27071,
                40647: 27073,
                40648: 27091,
                40649: 27070,
                40650: 27086,
                40651: 23528,
                40652: 27082,
                40653: 27101,
                40654: 27067,
                40655: 27075,
                40656: 27047,
                40657: 27182,
                40658: 27025,
                40659: 27040,
                40660: 27036,
                40661: 27029,
                40662: 27060,
                40663: 27102,
                40664: 27112,
                40665: 27138,
                40666: 27163,
                40667: 27135,
                40668: 27402,
                40669: 27129,
                40670: 27122,
                40671: 27111,
                40672: 27141,
                40673: 27057,
                40674: 27166,
                40675: 27117,
                40676: 27156,
                40677: 27115,
                40678: 27146,
                40679: 27154,
                40680: 27329,
                40681: 27171,
                40682: 27155,
                40683: 27204,
                40684: 27148,
                40685: 27250,
                40686: 27190,
                40687: 27256,
                40688: 27207,
                40689: 27234,
                40690: 27225,
                40691: 27238,
                40692: 27208,
                40693: 27192,
                40694: 27170,
                40695: 27280,
                40696: 27277,
                40697: 27296,
                40698: 27268,
                40699: 27298,
                40700: 27299,
                40768: 27287,
                40769: 34327,
                40770: 27323,
                40771: 27331,
                40772: 27330,
                40773: 27320,
                40774: 27315,
                40775: 27308,
                40776: 27358,
                40777: 27345,
                40778: 27359,
                40779: 27306,
                40780: 27354,
                40781: 27370,
                40782: 27387,
                40783: 27397,
                40784: 34326,
                40785: 27386,
                40786: 27410,
                40787: 27414,
                40788: 39729,
                40789: 27423,
                40790: 27448,
                40791: 27447,
                40792: 30428,
                40793: 27449,
                40794: 39150,
                40795: 27463,
                40796: 27459,
                40797: 27465,
                40798: 27472,
                40799: 27481,
                40800: 27476,
                40801: 27483,
                40802: 27487,
                40803: 27489,
                40804: 27512,
                40805: 27513,
                40806: 27519,
                40807: 27520,
                40808: 27524,
                40809: 27523,
                40810: 27533,
                40811: 27544,
                40812: 27541,
                40813: 27550,
                40814: 27556,
                40815: 27562,
                40816: 27563,
                40817: 27567,
                40818: 27570,
                40819: 27569,
                40820: 27571,
                40821: 27575,
                40822: 27580,
                40823: 27590,
                40824: 27595,
                40825: 27603,
                40826: 27615,
                40827: 27628,
                40828: 27627,
                40829: 27635,
                40830: 27631,
                40832: 40638,
                40833: 27656,
                40834: 27667,
                40835: 27668,
                40836: 27675,
                40837: 27684,
                40838: 27683,
                40839: 27742,
                40840: 27733,
                40841: 27746,
                40842: 27754,
                40843: 27778,
                40844: 27789,
                40845: 27802,
                40846: 27777,
                40847: 27803,
                40848: 27774,
                40849: 27752,
                40850: 27763,
                40851: 27794,
                40852: 27792,
                40853: 27844,
                40854: 27889,
                40855: 27859,
                40856: 27837,
                40857: 27863,
                40858: 27845,
                40859: 27869,
                40860: 27822,
                40861: 27825,
                40862: 27838,
                40863: 27834,
                40864: 27867,
                40865: 27887,
                40866: 27865,
                40867: 27882,
                40868: 27935,
                40869: 34893,
                40870: 27958,
                40871: 27947,
                40872: 27965,
                40873: 27960,
                40874: 27929,
                40875: 27957,
                40876: 27955,
                40877: 27922,
                40878: 27916,
                40879: 28003,
                40880: 28051,
                40881: 28004,
                40882: 27994,
                40883: 28025,
                40884: 27993,
                40885: 28046,
                40886: 28053,
                40887: 28644,
                40888: 28037,
                40889: 28153,
                40890: 28181,
                40891: 28170,
                40892: 28085,
                40893: 28103,
                40894: 28134,
                40895: 28088,
                40896: 28102,
                40897: 28140,
                40898: 28126,
                40899: 28108,
                40900: 28136,
                40901: 28114,
                40902: 28101,
                40903: 28154,
                40904: 28121,
                40905: 28132,
                40906: 28117,
                40907: 28138,
                40908: 28142,
                40909: 28205,
                40910: 28270,
                40911: 28206,
                40912: 28185,
                40913: 28274,
                40914: 28255,
                40915: 28222,
                40916: 28195,
                40917: 28267,
                40918: 28203,
                40919: 28278,
                40920: 28237,
                40921: 28191,
                40922: 28227,
                40923: 28218,
                40924: 28238,
                40925: 28196,
                40926: 28415,
                40927: 28189,
                40928: 28216,
                40929: 28290,
                40930: 28330,
                40931: 28312,
                40932: 28361,
                40933: 28343,
                40934: 28371,
                40935: 28349,
                40936: 28335,
                40937: 28356,
                40938: 28338,
                40939: 28372,
                40940: 28373,
                40941: 28303,
                40942: 28325,
                40943: 28354,
                40944: 28319,
                40945: 28481,
                40946: 28433,
                40947: 28748,
                40948: 28396,
                40949: 28408,
                40950: 28414,
                40951: 28479,
                40952: 28402,
                40953: 28465,
                40954: 28399,
                40955: 28466,
                40956: 28364,
                161: 65377,
                162: 65378,
                163: 65379,
                164: 65380,
                165: 65381,
                166: 65382,
                167: 65383,
                168: 65384,
                169: 65385,
                170: 65386,
                171: 65387,
                172: 65388,
                173: 65389,
                174: 65390,
                175: 65391,
                176: 65392,
                177: 65393,
                178: 65394,
                179: 65395,
                180: 65396,
                181: 65397,
                182: 65398,
                183: 65399,
                184: 65400,
                185: 65401,
                186: 65402,
                187: 65403,
                188: 65404,
                189: 65405,
                190: 65406,
                191: 65407,
                192: 65408,
                193: 65409,
                194: 65410,
                195: 65411,
                196: 65412,
                197: 65413,
                198: 65414,
                199: 65415,
                200: 65416,
                201: 65417,
                202: 65418,
                203: 65419,
                204: 65420,
                205: 65421,
                206: 65422,
                207: 65423,
                208: 65424,
                209: 65425,
                210: 65426,
                211: 65427,
                212: 65428,
                213: 65429,
                214: 65430,
                215: 65431,
                216: 65432,
                217: 65433,
                218: 65434,
                219: 65435,
                220: 65436,
                221: 65437,
                222: 65438,
                223: 65439,
                57408: 28478,
                57409: 28435,
                57410: 28407,
                57411: 28550,
                57412: 28538,
                57413: 28536,
                57414: 28545,
                57415: 28544,
                57416: 28527,
                57417: 28507,
                57418: 28659,
                57419: 28525,
                57420: 28546,
                57421: 28540,
                57422: 28504,
                57423: 28558,
                57424: 28561,
                57425: 28610,
                57426: 28518,
                57427: 28595,
                57428: 28579,
                57429: 28577,
                57430: 28580,
                57431: 28601,
                57432: 28614,
                57433: 28586,
                57434: 28639,
                57435: 28629,
                57436: 28652,
                57437: 28628,
                57438: 28632,
                57439: 28657,
                57440: 28654,
                57441: 28635,
                57442: 28681,
                57443: 28683,
                57444: 28666,
                57445: 28689,
                57446: 28673,
                57447: 28687,
                57448: 28670,
                57449: 28699,
                57450: 28698,
                57451: 28532,
                57452: 28701,
                57453: 28696,
                57454: 28703,
                57455: 28720,
                57456: 28734,
                57457: 28722,
                57458: 28753,
                57459: 28771,
                57460: 28825,
                57461: 28818,
                57462: 28847,
                57463: 28913,
                57464: 28844,
                57465: 28856,
                57466: 28851,
                57467: 28846,
                57468: 28895,
                57469: 28875,
                57470: 28893,
                57472: 28889,
                57473: 28937,
                57474: 28925,
                57475: 28956,
                57476: 28953,
                57477: 29029,
                57478: 29013,
                57479: 29064,
                57480: 29030,
                57481: 29026,
                57482: 29004,
                57483: 29014,
                57484: 29036,
                57485: 29071,
                57486: 29179,
                57487: 29060,
                57488: 29077,
                57489: 29096,
                57490: 29100,
                57491: 29143,
                57492: 29113,
                57493: 29118,
                57494: 29138,
                57495: 29129,
                57496: 29140,
                57497: 29134,
                57498: 29152,
                57499: 29164,
                57500: 29159,
                57501: 29173,
                57502: 29180,
                57503: 29177,
                57504: 29183,
                57505: 29197,
                57506: 29200,
                57507: 29211,
                57508: 29224,
                57509: 29229,
                57510: 29228,
                57511: 29232,
                57512: 29234,
                57513: 29243,
                57514: 29244,
                57515: 29247,
                57516: 29248,
                57517: 29254,
                57518: 29259,
                57519: 29272,
                57520: 29300,
                57521: 29310,
                57522: 29314,
                57523: 29313,
                57524: 29319,
                57525: 29330,
                57526: 29334,
                57527: 29346,
                57528: 29351,
                57529: 29369,
                57530: 29362,
                57531: 29379,
                57532: 29382,
                57533: 29380,
                57534: 29390,
                57535: 29394,
                57536: 29410,
                57537: 29408,
                57538: 29409,
                57539: 29433,
                57540: 29431,
                57541: 20495,
                57542: 29463,
                57543: 29450,
                57544: 29468,
                57545: 29462,
                57546: 29469,
                57547: 29492,
                57548: 29487,
                57549: 29481,
                57550: 29477,
                57551: 29502,
                57552: 29518,
                57553: 29519,
                57554: 40664,
                57555: 29527,
                57556: 29546,
                57557: 29544,
                57558: 29552,
                57559: 29560,
                57560: 29557,
                57561: 29563,
                57562: 29562,
                57563: 29640,
                57564: 29619,
                57565: 29646,
                57566: 29627,
                57567: 29632,
                57568: 29669,
                57569: 29678,
                57570: 29662,
                57571: 29858,
                57572: 29701,
                57573: 29807,
                57574: 29733,
                57575: 29688,
                57576: 29746,
                57577: 29754,
                57578: 29781,
                57579: 29759,
                57580: 29791,
                57581: 29785,
                57582: 29761,
                57583: 29788,
                57584: 29801,
                57585: 29808,
                57586: 29795,
                57587: 29802,
                57588: 29814,
                57589: 29822,
                57590: 29835,
                57591: 29854,
                57592: 29863,
                57593: 29898,
                57594: 29903,
                57595: 29908,
                57596: 29681,
                57664: 29920,
                57665: 29923,
                57666: 29927,
                57667: 29929,
                57668: 29934,
                57669: 29938,
                57670: 29936,
                57671: 29937,
                57672: 29944,
                57673: 29943,
                57674: 29956,
                57675: 29955,
                57676: 29957,
                57677: 29964,
                57678: 29966,
                57679: 29965,
                57680: 29973,
                57681: 29971,
                57682: 29982,
                57683: 29990,
                57684: 29996,
                57685: 30012,
                57686: 30020,
                57687: 30029,
                57688: 30026,
                57689: 30025,
                57690: 30043,
                57691: 30022,
                57692: 30042,
                57693: 30057,
                57694: 30052,
                57695: 30055,
                57696: 30059,
                57697: 30061,
                57698: 30072,
                57699: 30070,
                57700: 30086,
                57701: 30087,
                57702: 30068,
                57703: 30090,
                57704: 30089,
                57705: 30082,
                57706: 30100,
                57707: 30106,
                57708: 30109,
                57709: 30117,
                57710: 30115,
                57711: 30146,
                57712: 30131,
                57713: 30147,
                57714: 30133,
                57715: 30141,
                57716: 30136,
                57717: 30140,
                57718: 30129,
                57719: 30157,
                57720: 30154,
                57721: 30162,
                57722: 30169,
                57723: 30179,
                57724: 30174,
                57725: 30206,
                57726: 30207,
                57728: 30204,
                57729: 30209,
                57730: 30192,
                57731: 30202,
                57732: 30194,
                57733: 30195,
                57734: 30219,
                57735: 30221,
                57736: 30217,
                57737: 30239,
                57738: 30247,
                57739: 30240,
                57740: 30241,
                57741: 30242,
                57742: 30244,
                57743: 30260,
                57744: 30256,
                57745: 30267,
                57746: 30279,
                57747: 30280,
                57748: 30278,
                57749: 30300,
                57750: 30296,
                57751: 30305,
                57752: 30306,
                57753: 30312,
                57754: 30313,
                57755: 30314,
                57756: 30311,
                57757: 30316,
                57758: 30320,
                57759: 30322,
                57760: 30326,
                57761: 30328,
                57762: 30332,
                57763: 30336,
                57764: 30339,
                57765: 30344,
                57766: 30347,
                57767: 30350,
                57768: 30358,
                57769: 30355,
                57770: 30361,
                57771: 30362,
                57772: 30384,
                57773: 30388,
                57774: 30392,
                57775: 30393,
                57776: 30394,
                57777: 30402,
                57778: 30413,
                57779: 30422,
                57780: 30418,
                57781: 30430,
                57782: 30433,
                57783: 30437,
                57784: 30439,
                57785: 30442,
                57786: 34351,
                57787: 30459,
                57788: 30472,
                57789: 30471,
                57790: 30468,
                57791: 30505,
                57792: 30500,
                57793: 30494,
                57794: 30501,
                57795: 30502,
                57796: 30491,
                57797: 30519,
                57798: 30520,
                57799: 30535,
                57800: 30554,
                57801: 30568,
                57802: 30571,
                57803: 30555,
                57804: 30565,
                57805: 30591,
                57806: 30590,
                57807: 30585,
                57808: 30606,
                57809: 30603,
                57810: 30609,
                57811: 30624,
                57812: 30622,
                57813: 30640,
                57814: 30646,
                57815: 30649,
                57816: 30655,
                57817: 30652,
                57818: 30653,
                57819: 30651,
                57820: 30663,
                57821: 30669,
                57822: 30679,
                57823: 30682,
                57824: 30684,
                57825: 30691,
                57826: 30702,
                57827: 30716,
                57828: 30732,
                57829: 30738,
                57830: 31014,
                57831: 30752,
                57832: 31018,
                57833: 30789,
                57834: 30862,
                57835: 30836,
                57836: 30854,
                57837: 30844,
                57838: 30874,
                57839: 30860,
                57840: 30883,
                57841: 30901,
                57842: 30890,
                57843: 30895,
                57844: 30929,
                57845: 30918,
                57846: 30923,
                57847: 30932,
                57848: 30910,
                57849: 30908,
                57850: 30917,
                57851: 30922,
                57852: 30956,
                57920: 30951,
                57921: 30938,
                57922: 30973,
                57923: 30964,
                57924: 30983,
                57925: 30994,
                57926: 30993,
                57927: 31001,
                57928: 31020,
                57929: 31019,
                57930: 31040,
                57931: 31072,
                57932: 31063,
                57933: 31071,
                57934: 31066,
                57935: 31061,
                57936: 31059,
                57937: 31098,
                57938: 31103,
                57939: 31114,
                57940: 31133,
                57941: 31143,
                57942: 40779,
                57943: 31146,
                57944: 31150,
                57945: 31155,
                57946: 31161,
                57947: 31162,
                57948: 31177,
                57949: 31189,
                57950: 31207,
                57951: 31212,
                57952: 31201,
                57953: 31203,
                57954: 31240,
                57955: 31245,
                57956: 31256,
                57957: 31257,
                57958: 31264,
                57959: 31263,
                57960: 31104,
                57961: 31281,
                57962: 31291,
                57963: 31294,
                57964: 31287,
                57965: 31299,
                57966: 31319,
                57967: 31305,
                57968: 31329,
                57969: 31330,
                57970: 31337,
                57971: 40861,
                57972: 31344,
                57973: 31353,
                57974: 31357,
                57975: 31368,
                57976: 31383,
                57977: 31381,
                57978: 31384,
                57979: 31382,
                57980: 31401,
                57981: 31432,
                57982: 31408,
                57984: 31414,
                57985: 31429,
                57986: 31428,
                57987: 31423,
                57988: 36995,
                57989: 31431,
                57990: 31434,
                57991: 31437,
                57992: 31439,
                57993: 31445,
                57994: 31443,
                57995: 31449,
                57996: 31450,
                57997: 31453,
                57998: 31457,
                57999: 31458,
                58e3: 31462,
                58001: 31469,
                58002: 31472,
                58003: 31490,
                58004: 31503,
                58005: 31498,
                58006: 31494,
                58007: 31539,
                58008: 31512,
                58009: 31513,
                58010: 31518,
                58011: 31541,
                58012: 31528,
                58013: 31542,
                58014: 31568,
                58015: 31610,
                58016: 31492,
                58017: 31565,
                58018: 31499,
                58019: 31564,
                58020: 31557,
                58021: 31605,
                58022: 31589,
                58023: 31604,
                58024: 31591,
                58025: 31600,
                58026: 31601,
                58027: 31596,
                58028: 31598,
                58029: 31645,
                58030: 31640,
                58031: 31647,
                58032: 31629,
                58033: 31644,
                58034: 31642,
                58035: 31627,
                58036: 31634,
                58037: 31631,
                58038: 31581,
                58039: 31641,
                58040: 31691,
                58041: 31681,
                58042: 31692,
                58043: 31695,
                58044: 31668,
                58045: 31686,
                58046: 31709,
                58047: 31721,
                58048: 31761,
                58049: 31764,
                58050: 31718,
                58051: 31717,
                58052: 31840,
                58053: 31744,
                58054: 31751,
                58055: 31763,
                58056: 31731,
                58057: 31735,
                58058: 31767,
                58059: 31757,
                58060: 31734,
                58061: 31779,
                58062: 31783,
                58063: 31786,
                58064: 31775,
                58065: 31799,
                58066: 31787,
                58067: 31805,
                58068: 31820,
                58069: 31811,
                58070: 31828,
                58071: 31823,
                58072: 31808,
                58073: 31824,
                58074: 31832,
                58075: 31839,
                58076: 31844,
                58077: 31830,
                58078: 31845,
                58079: 31852,
                58080: 31861,
                58081: 31875,
                58082: 31888,
                58083: 31908,
                58084: 31917,
                58085: 31906,
                58086: 31915,
                58087: 31905,
                58088: 31912,
                58089: 31923,
                58090: 31922,
                58091: 31921,
                58092: 31918,
                58093: 31929,
                58094: 31933,
                58095: 31936,
                58096: 31941,
                58097: 31938,
                58098: 31960,
                58099: 31954,
                58100: 31964,
                58101: 31970,
                58102: 39739,
                58103: 31983,
                58104: 31986,
                58105: 31988,
                58106: 31990,
                58107: 31994,
                58108: 32006,
                58176: 32002,
                58177: 32028,
                58178: 32021,
                58179: 32010,
                58180: 32069,
                58181: 32075,
                58182: 32046,
                58183: 32050,
                58184: 32063,
                58185: 32053,
                58186: 32070,
                58187: 32115,
                58188: 32086,
                58189: 32078,
                58190: 32114,
                58191: 32104,
                58192: 32110,
                58193: 32079,
                58194: 32099,
                58195: 32147,
                58196: 32137,
                58197: 32091,
                58198: 32143,
                58199: 32125,
                58200: 32155,
                58201: 32186,
                58202: 32174,
                58203: 32163,
                58204: 32181,
                58205: 32199,
                58206: 32189,
                58207: 32171,
                58208: 32317,
                58209: 32162,
                58210: 32175,
                58211: 32220,
                58212: 32184,
                58213: 32159,
                58214: 32176,
                58215: 32216,
                58216: 32221,
                58217: 32228,
                58218: 32222,
                58219: 32251,
                58220: 32242,
                58221: 32225,
                58222: 32261,
                58223: 32266,
                58224: 32291,
                58225: 32289,
                58226: 32274,
                58227: 32305,
                58228: 32287,
                58229: 32265,
                58230: 32267,
                58231: 32290,
                58232: 32326,
                58233: 32358,
                58234: 32315,
                58235: 32309,
                58236: 32313,
                58237: 32323,
                58238: 32311,
                58240: 32306,
                58241: 32314,
                58242: 32359,
                58243: 32349,
                58244: 32342,
                58245: 32350,
                58246: 32345,
                58247: 32346,
                58248: 32377,
                58249: 32362,
                58250: 32361,
                58251: 32380,
                58252: 32379,
                58253: 32387,
                58254: 32213,
                58255: 32381,
                58256: 36782,
                58257: 32383,
                58258: 32392,
                58259: 32393,
                58260: 32396,
                58261: 32402,
                58262: 32400,
                58263: 32403,
                58264: 32404,
                58265: 32406,
                58266: 32398,
                58267: 32411,
                58268: 32412,
                58269: 32568,
                58270: 32570,
                58271: 32581,
                58272: 32588,
                58273: 32589,
                58274: 32590,
                58275: 32592,
                58276: 32593,
                58277: 32597,
                58278: 32596,
                58279: 32600,
                58280: 32607,
                58281: 32608,
                58282: 32616,
                58283: 32617,
                58284: 32615,
                58285: 32632,
                58286: 32642,
                58287: 32646,
                58288: 32643,
                58289: 32648,
                58290: 32647,
                58291: 32652,
                58292: 32660,
                58293: 32670,
                58294: 32669,
                58295: 32666,
                58296: 32675,
                58297: 32687,
                58298: 32690,
                58299: 32697,
                58300: 32686,
                58301: 32694,
                58302: 32696,
                58303: 35697,
                58304: 32709,
                58305: 32710,
                58306: 32714,
                58307: 32725,
                58308: 32724,
                58309: 32737,
                58310: 32742,
                58311: 32745,
                58312: 32755,
                58313: 32761,
                58314: 39132,
                58315: 32774,
                58316: 32772,
                58317: 32779,
                58318: 32786,
                58319: 32792,
                58320: 32793,
                58321: 32796,
                58322: 32801,
                58323: 32808,
                58324: 32831,
                58325: 32827,
                58326: 32842,
                58327: 32838,
                58328: 32850,
                58329: 32856,
                58330: 32858,
                58331: 32863,
                58332: 32866,
                58333: 32872,
                58334: 32883,
                58335: 32882,
                58336: 32880,
                58337: 32886,
                58338: 32889,
                58339: 32893,
                58340: 32895,
                58341: 32900,
                58342: 32902,
                58343: 32901,
                58344: 32923,
                58345: 32915,
                58346: 32922,
                58347: 32941,
                58348: 20880,
                58349: 32940,
                58350: 32987,
                58351: 32997,
                58352: 32985,
                58353: 32989,
                58354: 32964,
                58355: 32986,
                58356: 32982,
                58357: 33033,
                58358: 33007,
                58359: 33009,
                58360: 33051,
                58361: 33065,
                58362: 33059,
                58363: 33071,
                58364: 33099,
                58432: 38539,
                58433: 33094,
                58434: 33086,
                58435: 33107,
                58436: 33105,
                58437: 33020,
                58438: 33137,
                58439: 33134,
                58440: 33125,
                58441: 33126,
                58442: 33140,
                58443: 33155,
                58444: 33160,
                58445: 33162,
                58446: 33152,
                58447: 33154,
                58448: 33184,
                58449: 33173,
                58450: 33188,
                58451: 33187,
                58452: 33119,
                58453: 33171,
                58454: 33193,
                58455: 33200,
                58456: 33205,
                58457: 33214,
                58458: 33208,
                58459: 33213,
                58460: 33216,
                58461: 33218,
                58462: 33210,
                58463: 33225,
                58464: 33229,
                58465: 33233,
                58466: 33241,
                58467: 33240,
                58468: 33224,
                58469: 33242,
                58470: 33247,
                58471: 33248,
                58472: 33255,
                58473: 33274,
                58474: 33275,
                58475: 33278,
                58476: 33281,
                58477: 33282,
                58478: 33285,
                58479: 33287,
                58480: 33290,
                58481: 33293,
                58482: 33296,
                58483: 33302,
                58484: 33321,
                58485: 33323,
                58486: 33336,
                58487: 33331,
                58488: 33344,
                58489: 33369,
                58490: 33368,
                58491: 33373,
                58492: 33370,
                58493: 33375,
                58494: 33380,
                58496: 33378,
                58497: 33384,
                58498: 33386,
                58499: 33387,
                58500: 33326,
                58501: 33393,
                58502: 33399,
                58503: 33400,
                58504: 33406,
                58505: 33421,
                58506: 33426,
                58507: 33451,
                58508: 33439,
                58509: 33467,
                58510: 33452,
                58511: 33505,
                58512: 33507,
                58513: 33503,
                58514: 33490,
                58515: 33524,
                58516: 33523,
                58517: 33530,
                58518: 33683,
                58519: 33539,
                58520: 33531,
                58521: 33529,
                58522: 33502,
                58523: 33542,
                58524: 33500,
                58525: 33545,
                58526: 33497,
                58527: 33589,
                58528: 33588,
                58529: 33558,
                58530: 33586,
                58531: 33585,
                58532: 33600,
                58533: 33593,
                58534: 33616,
                58535: 33605,
                58536: 33583,
                58537: 33579,
                58538: 33559,
                58539: 33560,
                58540: 33669,
                58541: 33690,
                58542: 33706,
                58543: 33695,
                58544: 33698,
                58545: 33686,
                58546: 33571,
                58547: 33678,
                58548: 33671,
                58549: 33674,
                58550: 33660,
                58551: 33717,
                58552: 33651,
                58553: 33653,
                58554: 33696,
                58555: 33673,
                58556: 33704,
                58557: 33780,
                58558: 33811,
                58559: 33771,
                58560: 33742,
                58561: 33789,
                58562: 33795,
                58563: 33752,
                58564: 33803,
                58565: 33729,
                58566: 33783,
                58567: 33799,
                58568: 33760,
                58569: 33778,
                58570: 33805,
                58571: 33826,
                58572: 33824,
                58573: 33725,
                58574: 33848,
                58575: 34054,
                58576: 33787,
                58577: 33901,
                58578: 33834,
                58579: 33852,
                58580: 34138,
                58581: 33924,
                58582: 33911,
                58583: 33899,
                58584: 33965,
                58585: 33902,
                58586: 33922,
                58587: 33897,
                58588: 33862,
                58589: 33836,
                58590: 33903,
                58591: 33913,
                58592: 33845,
                58593: 33994,
                58594: 33890,
                58595: 33977,
                58596: 33983,
                58597: 33951,
                58598: 34009,
                58599: 33997,
                58600: 33979,
                58601: 34010,
                58602: 34e3,
                58603: 33985,
                58604: 33990,
                58605: 34006,
                58606: 33953,
                58607: 34081,
                58608: 34047,
                58609: 34036,
                58610: 34071,
                58611: 34072,
                58612: 34092,
                58613: 34079,
                58614: 34069,
                58615: 34068,
                58616: 34044,
                58617: 34112,
                58618: 34147,
                58619: 34136,
                58620: 34120,
                58688: 34113,
                58689: 34306,
                58690: 34123,
                58691: 34133,
                58692: 34176,
                58693: 34212,
                58694: 34184,
                58695: 34193,
                58696: 34186,
                58697: 34216,
                58698: 34157,
                58699: 34196,
                58700: 34203,
                58701: 34282,
                58702: 34183,
                58703: 34204,
                58704: 34167,
                58705: 34174,
                58706: 34192,
                58707: 34249,
                58708: 34234,
                58709: 34255,
                58710: 34233,
                58711: 34256,
                58712: 34261,
                58713: 34269,
                58714: 34277,
                58715: 34268,
                58716: 34297,
                58717: 34314,
                58718: 34323,
                58719: 34315,
                58720: 34302,
                58721: 34298,
                58722: 34310,
                58723: 34338,
                58724: 34330,
                58725: 34352,
                58726: 34367,
                58727: 34381,
                58728: 20053,
                58729: 34388,
                58730: 34399,
                58731: 34407,
                58732: 34417,
                58733: 34451,
                58734: 34467,
                58735: 34473,
                58736: 34474,
                58737: 34443,
                58738: 34444,
                58739: 34486,
                58740: 34479,
                58741: 34500,
                58742: 34502,
                58743: 34480,
                58744: 34505,
                58745: 34851,
                58746: 34475,
                58747: 34516,
                58748: 34526,
                58749: 34537,
                58750: 34540,
                58752: 34527,
                58753: 34523,
                58754: 34543,
                58755: 34578,
                58756: 34566,
                58757: 34568,
                58758: 34560,
                58759: 34563,
                58760: 34555,
                58761: 34577,
                58762: 34569,
                58763: 34573,
                58764: 34553,
                58765: 34570,
                58766: 34612,
                58767: 34623,
                58768: 34615,
                58769: 34619,
                58770: 34597,
                58771: 34601,
                58772: 34586,
                58773: 34656,
                58774: 34655,
                58775: 34680,
                58776: 34636,
                58777: 34638,
                58778: 34676,
                58779: 34647,
                58780: 34664,
                58781: 34670,
                58782: 34649,
                58783: 34643,
                58784: 34659,
                58785: 34666,
                58786: 34821,
                58787: 34722,
                58788: 34719,
                58789: 34690,
                58790: 34735,
                58791: 34763,
                58792: 34749,
                58793: 34752,
                58794: 34768,
                58795: 38614,
                58796: 34731,
                58797: 34756,
                58798: 34739,
                58799: 34759,
                58800: 34758,
                58801: 34747,
                58802: 34799,
                58803: 34802,
                58804: 34784,
                58805: 34831,
                58806: 34829,
                58807: 34814,
                58808: 34806,
                58809: 34807,
                58810: 34830,
                58811: 34770,
                58812: 34833,
                58813: 34838,
                58814: 34837,
                58815: 34850,
                58816: 34849,
                58817: 34865,
                58818: 34870,
                58819: 34873,
                58820: 34855,
                58821: 34875,
                58822: 34884,
                58823: 34882,
                58824: 34898,
                58825: 34905,
                58826: 34910,
                58827: 34914,
                58828: 34923,
                58829: 34945,
                58830: 34942,
                58831: 34974,
                58832: 34933,
                58833: 34941,
                58834: 34997,
                58835: 34930,
                58836: 34946,
                58837: 34967,
                58838: 34962,
                58839: 34990,
                58840: 34969,
                58841: 34978,
                58842: 34957,
                58843: 34980,
                58844: 34992,
                58845: 35007,
                58846: 34993,
                58847: 35011,
                58848: 35012,
                58849: 35028,
                58850: 35032,
                58851: 35033,
                58852: 35037,
                58853: 35065,
                58854: 35074,
                58855: 35068,
                58856: 35060,
                58857: 35048,
                58858: 35058,
                58859: 35076,
                58860: 35084,
                58861: 35082,
                58862: 35091,
                58863: 35139,
                58864: 35102,
                58865: 35109,
                58866: 35114,
                58867: 35115,
                58868: 35137,
                58869: 35140,
                58870: 35131,
                58871: 35126,
                58872: 35128,
                58873: 35148,
                58874: 35101,
                58875: 35168,
                58876: 35166,
                58944: 35174,
                58945: 35172,
                58946: 35181,
                58947: 35178,
                58948: 35183,
                58949: 35188,
                58950: 35191,
                58951: 35198,
                58952: 35203,
                58953: 35208,
                58954: 35210,
                58955: 35219,
                58956: 35224,
                58957: 35233,
                58958: 35241,
                58959: 35238,
                58960: 35244,
                58961: 35247,
                58962: 35250,
                58963: 35258,
                58964: 35261,
                58965: 35263,
                58966: 35264,
                58967: 35290,
                58968: 35292,
                58969: 35293,
                58970: 35303,
                58971: 35316,
                58972: 35320,
                58973: 35331,
                58974: 35350,
                58975: 35344,
                58976: 35340,
                58977: 35355,
                58978: 35357,
                58979: 35365,
                58980: 35382,
                58981: 35393,
                58982: 35419,
                58983: 35410,
                58984: 35398,
                58985: 35400,
                58986: 35452,
                58987: 35437,
                58988: 35436,
                58989: 35426,
                58990: 35461,
                58991: 35458,
                58992: 35460,
                58993: 35496,
                58994: 35489,
                58995: 35473,
                58996: 35493,
                58997: 35494,
                58998: 35482,
                58999: 35491,
                59e3: 35524,
                59001: 35533,
                59002: 35522,
                59003: 35546,
                59004: 35563,
                59005: 35571,
                59006: 35559,
                59008: 35556,
                59009: 35569,
                59010: 35604,
                59011: 35552,
                59012: 35554,
                59013: 35575,
                59014: 35550,
                59015: 35547,
                59016: 35596,
                59017: 35591,
                59018: 35610,
                59019: 35553,
                59020: 35606,
                59021: 35600,
                59022: 35607,
                59023: 35616,
                59024: 35635,
                59025: 38827,
                59026: 35622,
                59027: 35627,
                59028: 35646,
                59029: 35624,
                59030: 35649,
                59031: 35660,
                59032: 35663,
                59033: 35662,
                59034: 35657,
                59035: 35670,
                59036: 35675,
                59037: 35674,
                59038: 35691,
                59039: 35679,
                59040: 35692,
                59041: 35695,
                59042: 35700,
                59043: 35709,
                59044: 35712,
                59045: 35724,
                59046: 35726,
                59047: 35730,
                59048: 35731,
                59049: 35734,
                59050: 35737,
                59051: 35738,
                59052: 35898,
                59053: 35905,
                59054: 35903,
                59055: 35912,
                59056: 35916,
                59057: 35918,
                59058: 35920,
                59059: 35925,
                59060: 35938,
                59061: 35948,
                59062: 35960,
                59063: 35962,
                59064: 35970,
                59065: 35977,
                59066: 35973,
                59067: 35978,
                59068: 35981,
                59069: 35982,
                59070: 35988,
                59071: 35964,
                59072: 35992,
                59073: 25117,
                59074: 36013,
                59075: 36010,
                59076: 36029,
                59077: 36018,
                59078: 36019,
                59079: 36014,
                59080: 36022,
                59081: 36040,
                59082: 36033,
                59083: 36068,
                59084: 36067,
                59085: 36058,
                59086: 36093,
                59087: 36090,
                59088: 36091,
                59089: 36100,
                59090: 36101,
                59091: 36106,
                59092: 36103,
                59093: 36111,
                59094: 36109,
                59095: 36112,
                59096: 40782,
                59097: 36115,
                59098: 36045,
                59099: 36116,
                59100: 36118,
                59101: 36199,
                59102: 36205,
                59103: 36209,
                59104: 36211,
                59105: 36225,
                59106: 36249,
                59107: 36290,
                59108: 36286,
                59109: 36282,
                59110: 36303,
                59111: 36314,
                59112: 36310,
                59113: 36300,
                59114: 36315,
                59115: 36299,
                59116: 36330,
                59117: 36331,
                59118: 36319,
                59119: 36323,
                59120: 36348,
                59121: 36360,
                59122: 36361,
                59123: 36351,
                59124: 36381,
                59125: 36382,
                59126: 36368,
                59127: 36383,
                59128: 36418,
                59129: 36405,
                59130: 36400,
                59131: 36404,
                59132: 36426,
                59200: 36423,
                59201: 36425,
                59202: 36428,
                59203: 36432,
                59204: 36424,
                59205: 36441,
                59206: 36452,
                59207: 36448,
                59208: 36394,
                59209: 36451,
                59210: 36437,
                59211: 36470,
                59212: 36466,
                59213: 36476,
                59214: 36481,
                59215: 36487,
                59216: 36485,
                59217: 36484,
                59218: 36491,
                59219: 36490,
                59220: 36499,
                59221: 36497,
                59222: 36500,
                59223: 36505,
                59224: 36522,
                59225: 36513,
                59226: 36524,
                59227: 36528,
                59228: 36550,
                59229: 36529,
                59230: 36542,
                59231: 36549,
                59232: 36552,
                59233: 36555,
                59234: 36571,
                59235: 36579,
                59236: 36604,
                59237: 36603,
                59238: 36587,
                59239: 36606,
                59240: 36618,
                59241: 36613,
                59242: 36629,
                59243: 36626,
                59244: 36633,
                59245: 36627,
                59246: 36636,
                59247: 36639,
                59248: 36635,
                59249: 36620,
                59250: 36646,
                59251: 36659,
                59252: 36667,
                59253: 36665,
                59254: 36677,
                59255: 36674,
                59256: 36670,
                59257: 36684,
                59258: 36681,
                59259: 36678,
                59260: 36686,
                59261: 36695,
                59262: 36700,
                59264: 36706,
                59265: 36707,
                59266: 36708,
                59267: 36764,
                59268: 36767,
                59269: 36771,
                59270: 36781,
                59271: 36783,
                59272: 36791,
                59273: 36826,
                59274: 36837,
                59275: 36834,
                59276: 36842,
                59277: 36847,
                59278: 36999,
                59279: 36852,
                59280: 36869,
                59281: 36857,
                59282: 36858,
                59283: 36881,
                59284: 36885,
                59285: 36897,
                59286: 36877,
                59287: 36894,
                59288: 36886,
                59289: 36875,
                59290: 36903,
                59291: 36918,
                59292: 36917,
                59293: 36921,
                59294: 36856,
                59295: 36943,
                59296: 36944,
                59297: 36945,
                59298: 36946,
                59299: 36878,
                59300: 36937,
                59301: 36926,
                59302: 36950,
                59303: 36952,
                59304: 36958,
                59305: 36968,
                59306: 36975,
                59307: 36982,
                59308: 38568,
                59309: 36978,
                59310: 36994,
                59311: 36989,
                59312: 36993,
                59313: 36992,
                59314: 37002,
                59315: 37001,
                59316: 37007,
                59317: 37032,
                59318: 37039,
                59319: 37041,
                59320: 37045,
                59321: 37090,
                59322: 37092,
                59323: 25160,
                59324: 37083,
                59325: 37122,
                59326: 37138,
                59327: 37145,
                59328: 37170,
                59329: 37168,
                59330: 37194,
                59331: 37206,
                59332: 37208,
                59333: 37219,
                59334: 37221,
                59335: 37225,
                59336: 37235,
                59337: 37234,
                59338: 37259,
                59339: 37257,
                59340: 37250,
                59341: 37282,
                59342: 37291,
                59343: 37295,
                59344: 37290,
                59345: 37301,
                59346: 37300,
                59347: 37306,
                59348: 37312,
                59349: 37313,
                59350: 37321,
                59351: 37323,
                59352: 37328,
                59353: 37334,
                59354: 37343,
                59355: 37345,
                59356: 37339,
                59357: 37372,
                59358: 37365,
                59359: 37366,
                59360: 37406,
                59361: 37375,
                59362: 37396,
                59363: 37420,
                59364: 37397,
                59365: 37393,
                59366: 37470,
                59367: 37463,
                59368: 37445,
                59369: 37449,
                59370: 37476,
                59371: 37448,
                59372: 37525,
                59373: 37439,
                59374: 37451,
                59375: 37456,
                59376: 37532,
                59377: 37526,
                59378: 37523,
                59379: 37531,
                59380: 37466,
                59381: 37583,
                59382: 37561,
                59383: 37559,
                59384: 37609,
                59385: 37647,
                59386: 37626,
                59387: 37700,
                59388: 37678,
                59456: 37657,
                59457: 37666,
                59458: 37658,
                59459: 37667,
                59460: 37690,
                59461: 37685,
                59462: 37691,
                59463: 37724,
                59464: 37728,
                59465: 37756,
                59466: 37742,
                59467: 37718,
                59468: 37808,
                59469: 37804,
                59470: 37805,
                59471: 37780,
                59472: 37817,
                59473: 37846,
                59474: 37847,
                59475: 37864,
                59476: 37861,
                59477: 37848,
                59478: 37827,
                59479: 37853,
                59480: 37840,
                59481: 37832,
                59482: 37860,
                59483: 37914,
                59484: 37908,
                59485: 37907,
                59486: 37891,
                59487: 37895,
                59488: 37904,
                59489: 37942,
                59490: 37931,
                59491: 37941,
                59492: 37921,
                59493: 37946,
                59494: 37953,
                59495: 37970,
                59496: 37956,
                59497: 37979,
                59498: 37984,
                59499: 37986,
                59500: 37982,
                59501: 37994,
                59502: 37417,
                59503: 38e3,
                59504: 38005,
                59505: 38007,
                59506: 38013,
                59507: 37978,
                59508: 38012,
                59509: 38014,
                59510: 38017,
                59511: 38015,
                59512: 38274,
                59513: 38279,
                59514: 38282,
                59515: 38292,
                59516: 38294,
                59517: 38296,
                59518: 38297,
                59520: 38304,
                59521: 38312,
                59522: 38311,
                59523: 38317,
                59524: 38332,
                59525: 38331,
                59526: 38329,
                59527: 38334,
                59528: 38346,
                59529: 28662,
                59530: 38339,
                59531: 38349,
                59532: 38348,
                59533: 38357,
                59534: 38356,
                59535: 38358,
                59536: 38364,
                59537: 38369,
                59538: 38373,
                59539: 38370,
                59540: 38433,
                59541: 38440,
                59542: 38446,
                59543: 38447,
                59544: 38466,
                59545: 38476,
                59546: 38479,
                59547: 38475,
                59548: 38519,
                59549: 38492,
                59550: 38494,
                59551: 38493,
                59552: 38495,
                59553: 38502,
                59554: 38514,
                59555: 38508,
                59556: 38541,
                59557: 38552,
                59558: 38549,
                59559: 38551,
                59560: 38570,
                59561: 38567,
                59562: 38577,
                59563: 38578,
                59564: 38576,
                59565: 38580,
                59566: 38582,
                59567: 38584,
                59568: 38585,
                59569: 38606,
                59570: 38603,
                59571: 38601,
                59572: 38605,
                59573: 35149,
                59574: 38620,
                59575: 38669,
                59576: 38613,
                59577: 38649,
                59578: 38660,
                59579: 38662,
                59580: 38664,
                59581: 38675,
                59582: 38670,
                59583: 38673,
                59584: 38671,
                59585: 38678,
                59586: 38681,
                59587: 38692,
                59588: 38698,
                59589: 38704,
                59590: 38713,
                59591: 38717,
                59592: 38718,
                59593: 38724,
                59594: 38726,
                59595: 38728,
                59596: 38722,
                59597: 38729,
                59598: 38748,
                59599: 38752,
                59600: 38756,
                59601: 38758,
                59602: 38760,
                59603: 21202,
                59604: 38763,
                59605: 38769,
                59606: 38777,
                59607: 38789,
                59608: 38780,
                59609: 38785,
                59610: 38778,
                59611: 38790,
                59612: 38795,
                59613: 38799,
                59614: 38800,
                59615: 38812,
                59616: 38824,
                59617: 38822,
                59618: 38819,
                59619: 38835,
                59620: 38836,
                59621: 38851,
                59622: 38854,
                59623: 38856,
                59624: 38859,
                59625: 38876,
                59626: 38893,
                59627: 40783,
                59628: 38898,
                59629: 31455,
                59630: 38902,
                59631: 38901,
                59632: 38927,
                59633: 38924,
                59634: 38968,
                59635: 38948,
                59636: 38945,
                59637: 38967,
                59638: 38973,
                59639: 38982,
                59640: 38991,
                59641: 38987,
                59642: 39019,
                59643: 39023,
                59644: 39024,
                59712: 39025,
                59713: 39028,
                59714: 39027,
                59715: 39082,
                59716: 39087,
                59717: 39089,
                59718: 39094,
                59719: 39108,
                59720: 39107,
                59721: 39110,
                59722: 39145,
                59723: 39147,
                59724: 39171,
                59725: 39177,
                59726: 39186,
                59727: 39188,
                59728: 39192,
                59729: 39201,
                59730: 39197,
                59731: 39198,
                59732: 39204,
                59733: 39200,
                59734: 39212,
                59735: 39214,
                59736: 39229,
                59737: 39230,
                59738: 39234,
                59739: 39241,
                59740: 39237,
                59741: 39248,
                59742: 39243,
                59743: 39249,
                59744: 39250,
                59745: 39244,
                59746: 39253,
                59747: 39319,
                59748: 39320,
                59749: 39333,
                59750: 39341,
                59751: 39342,
                59752: 39356,
                59753: 39391,
                59754: 39387,
                59755: 39389,
                59756: 39384,
                59757: 39377,
                59758: 39405,
                59759: 39406,
                59760: 39409,
                59761: 39410,
                59762: 39419,
                59763: 39416,
                59764: 39425,
                59765: 39439,
                59766: 39429,
                59767: 39394,
                59768: 39449,
                59769: 39467,
                59770: 39479,
                59771: 39493,
                59772: 39490,
                59773: 39488,
                59774: 39491,
                59776: 39486,
                59777: 39509,
                59778: 39501,
                59779: 39515,
                59780: 39511,
                59781: 39519,
                59782: 39522,
                59783: 39525,
                59784: 39524,
                59785: 39529,
                59786: 39531,
                59787: 39530,
                59788: 39597,
                59789: 39600,
                59790: 39612,
                59791: 39616,
                59792: 39631,
                59793: 39633,
                59794: 39635,
                59795: 39636,
                59796: 39646,
                59797: 39647,
                59798: 39650,
                59799: 39651,
                59800: 39654,
                59801: 39663,
                59802: 39659,
                59803: 39662,
                59804: 39668,
                59805: 39665,
                59806: 39671,
                59807: 39675,
                59808: 39686,
                59809: 39704,
                59810: 39706,
                59811: 39711,
                59812: 39714,
                59813: 39715,
                59814: 39717,
                59815: 39719,
                59816: 39720,
                59817: 39721,
                59818: 39722,
                59819: 39726,
                59820: 39727,
                59821: 39730,
                59822: 39748,
                59823: 39747,
                59824: 39759,
                59825: 39757,
                59826: 39758,
                59827: 39761,
                59828: 39768,
                59829: 39796,
                59830: 39827,
                59831: 39811,
                59832: 39825,
                59833: 39830,
                59834: 39831,
                59835: 39839,
                59836: 39840,
                59837: 39848,
                59838: 39860,
                59839: 39872,
                59840: 39882,
                59841: 39865,
                59842: 39878,
                59843: 39887,
                59844: 39889,
                59845: 39890,
                59846: 39907,
                59847: 39906,
                59848: 39908,
                59849: 39892,
                59850: 39905,
                59851: 39994,
                59852: 39922,
                59853: 39921,
                59854: 39920,
                59855: 39957,
                59856: 39956,
                59857: 39945,
                59858: 39955,
                59859: 39948,
                59860: 39942,
                59861: 39944,
                59862: 39954,
                59863: 39946,
                59864: 39940,
                59865: 39982,
                59866: 39963,
                59867: 39973,
                59868: 39972,
                59869: 39969,
                59870: 39984,
                59871: 40007,
                59872: 39986,
                59873: 40006,
                59874: 39998,
                59875: 40026,
                59876: 40032,
                59877: 40039,
                59878: 40054,
                59879: 40056,
                59880: 40167,
                59881: 40172,
                59882: 40176,
                59883: 40201,
                59884: 40200,
                59885: 40171,
                59886: 40195,
                59887: 40198,
                59888: 40234,
                59889: 40230,
                59890: 40367,
                59891: 40227,
                59892: 40223,
                59893: 40260,
                59894: 40213,
                59895: 40210,
                59896: 40257,
                59897: 40255,
                59898: 40254,
                59899: 40262,
                59900: 40264,
                59968: 40285,
                59969: 40286,
                59970: 40292,
                59971: 40273,
                59972: 40272,
                59973: 40281,
                59974: 40306,
                59975: 40329,
                59976: 40327,
                59977: 40363,
                59978: 40303,
                59979: 40314,
                59980: 40346,
                59981: 40356,
                59982: 40361,
                59983: 40370,
                59984: 40388,
                59985: 40385,
                59986: 40379,
                59987: 40376,
                59988: 40378,
                59989: 40390,
                59990: 40399,
                59991: 40386,
                59992: 40409,
                59993: 40403,
                59994: 40440,
                59995: 40422,
                59996: 40429,
                59997: 40431,
                59998: 40445,
                59999: 40474,
                6e4: 40475,
                60001: 40478,
                60002: 40565,
                60003: 40569,
                60004: 40573,
                60005: 40577,
                60006: 40584,
                60007: 40587,
                60008: 40588,
                60009: 40594,
                60010: 40597,
                60011: 40593,
                60012: 40605,
                60013: 40613,
                60014: 40617,
                60015: 40632,
                60016: 40618,
                60017: 40621,
                60018: 38753,
                60019: 40652,
                60020: 40654,
                60021: 40655,
                60022: 40656,
                60023: 40660,
                60024: 40668,
                60025: 40670,
                60026: 40669,
                60027: 40672,
                60028: 40677,
                60029: 40680,
                60030: 40687,
                60032: 40692,
                60033: 40694,
                60034: 40695,
                60035: 40697,
                60036: 40699,
                60037: 40700,
                60038: 40701,
                60039: 40711,
                60040: 40712,
                60041: 30391,
                60042: 40725,
                60043: 40737,
                60044: 40748,
                60045: 40766,
                60046: 40778,
                60047: 40786,
                60048: 40788,
                60049: 40803,
                60050: 40799,
                60051: 40800,
                60052: 40801,
                60053: 40806,
                60054: 40807,
                60055: 40812,
                60056: 40810,
                60057: 40823,
                60058: 40818,
                60059: 40822,
                60060: 40853,
                60061: 40860,
                60062: 40864,
                60063: 22575,
                60064: 27079,
                60065: 36953,
                60066: 29796,
                60067: 20956,
                60068: 29081
              };
            }),
            /* 9 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var GenericGF_1 = __webpack_require__(1), GenericGFPoly_1 = __webpack_require__(2);
              function runEuclideanAlgorithm(field, a, b, R) {
                var _a;
                a.degree() < b.degree() && (_a = [b, a], a = _a[0], b = _a[1]);
                for (var rLast = a, r = b, tLast = field.zero, t = field.one; r.degree() >= R / 2; ) {
                  var rLastLast = rLast, tLastLast = tLast;
                  if (rLast = r, tLast = t, rLast.isZero())
                    return null;
                  r = rLastLast;
                  for (var q = field.zero, denominatorLeadingTerm = rLast.getCoefficient(rLast.degree()), dltInverse = field.inverse(denominatorLeadingTerm); r.degree() >= rLast.degree() && !r.isZero(); ) {
                    var degreeDiff = r.degree() - rLast.degree(), scale = field.multiply(r.getCoefficient(r.degree()), dltInverse);
                    q = q.addOrSubtract(field.buildMonomial(degreeDiff, scale)), r = r.addOrSubtract(rLast.multiplyByMonomial(degreeDiff, scale));
                  }
                  if (t = q.multiplyPoly(tLast).addOrSubtract(tLastLast), r.degree() >= rLast.degree())
                    return null;
                }
                var sigmaTildeAtZero = t.getCoefficient(0);
                if (sigmaTildeAtZero === 0)
                  return null;
                var inverse = field.inverse(sigmaTildeAtZero);
                return [t.multiply(inverse), r.multiply(inverse)];
              }
              function findErrorLocations(field, errorLocator) {
                var numErrors = errorLocator.degree();
                if (numErrors === 1)
                  return [errorLocator.getCoefficient(1)];
                for (var result = new Array(numErrors), errorCount = 0, i = 1; i < field.size && errorCount < numErrors; i++)
                  errorLocator.evaluateAt(i) === 0 && (result[errorCount] = field.inverse(i), errorCount++);
                return errorCount !== numErrors ? null : result;
              }
              function findErrorMagnitudes(field, errorEvaluator, errorLocations) {
                for (var s = errorLocations.length, result = new Array(s), i = 0; i < s; i++) {
                  for (var xiInverse = field.inverse(errorLocations[i]), denominator = 1, j = 0; j < s; j++)
                    i !== j && (denominator = field.multiply(denominator, GenericGF_1.addOrSubtractGF(1, field.multiply(errorLocations[j], xiInverse))));
                  result[i] = field.multiply(errorEvaluator.evaluateAt(xiInverse), field.inverse(denominator)), field.generatorBase !== 0 && (result[i] = field.multiply(result[i], xiInverse));
                }
                return result;
              }
              function decode(bytes, twoS) {
                var outputBytes = new Uint8ClampedArray(bytes.length);
                outputBytes.set(bytes);
                for (var field = new GenericGF_1.default(285, 256, 0), poly = new GenericGFPoly_1.default(field, outputBytes), syndromeCoefficients = new Uint8ClampedArray(twoS), error = !1, s = 0; s < twoS; s++) {
                  var evaluation = poly.evaluateAt(field.exp(s + field.generatorBase));
                  syndromeCoefficients[syndromeCoefficients.length - 1 - s] = evaluation, evaluation !== 0 && (error = !0);
                }
                if (!error)
                  return outputBytes;
                var syndrome = new GenericGFPoly_1.default(field, syndromeCoefficients), sigmaOmega = runEuclideanAlgorithm(field, field.buildMonomial(twoS, 1), syndrome, twoS);
                if (sigmaOmega === null)
                  return null;
                var errorLocations = findErrorLocations(field, sigmaOmega[0]);
                if (errorLocations == null)
                  return null;
                for (var errorMagnitudes = findErrorMagnitudes(field, sigmaOmega[1], errorLocations), i = 0; i < errorLocations.length; i++) {
                  var position = outputBytes.length - 1 - field.log(errorLocations[i]);
                  if (position < 0)
                    return null;
                  outputBytes[position] = GenericGF_1.addOrSubtractGF(outputBytes[position], errorMagnitudes[i]);
                }
                return outputBytes;
              }
              exports2.decode = decode;
            }),
            /* 10 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 }), exports2.VERSIONS = [
                {
                  infoBits: null,
                  versionNumber: 1,
                  alignmentPatternCenters: [],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 7,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 19 }]
                    },
                    {
                      ecCodewordsPerBlock: 10,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 16 }]
                    },
                    {
                      ecCodewordsPerBlock: 13,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 13 }]
                    },
                    {
                      ecCodewordsPerBlock: 17,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 9 }]
                    }
                  ]
                },
                {
                  infoBits: null,
                  versionNumber: 2,
                  alignmentPatternCenters: [6, 18],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 10,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 34 }]
                    },
                    {
                      ecCodewordsPerBlock: 16,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 28 }]
                    },
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 22 }]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 16 }]
                    }
                  ]
                },
                {
                  infoBits: null,
                  versionNumber: 3,
                  alignmentPatternCenters: [6, 22],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 15,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 55 }]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 44 }]
                    },
                    {
                      ecCodewordsPerBlock: 18,
                      ecBlocks: [{ numBlocks: 2, dataCodewordsPerBlock: 17 }]
                    },
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [{ numBlocks: 2, dataCodewordsPerBlock: 13 }]
                    }
                  ]
                },
                {
                  infoBits: null,
                  versionNumber: 4,
                  alignmentPatternCenters: [6, 26],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 20,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 80 }]
                    },
                    {
                      ecCodewordsPerBlock: 18,
                      ecBlocks: [{ numBlocks: 2, dataCodewordsPerBlock: 32 }]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [{ numBlocks: 2, dataCodewordsPerBlock: 24 }]
                    },
                    {
                      ecCodewordsPerBlock: 16,
                      ecBlocks: [{ numBlocks: 4, dataCodewordsPerBlock: 9 }]
                    }
                  ]
                },
                {
                  infoBits: null,
                  versionNumber: 5,
                  alignmentPatternCenters: [6, 30],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [{ numBlocks: 1, dataCodewordsPerBlock: 108 }]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [{ numBlocks: 2, dataCodewordsPerBlock: 43 }]
                    },
                    {
                      ecCodewordsPerBlock: 18,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 15 },
                        { numBlocks: 2, dataCodewordsPerBlock: 16 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 11 },
                        { numBlocks: 2, dataCodewordsPerBlock: 12 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: null,
                  versionNumber: 6,
                  alignmentPatternCenters: [6, 34],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 18,
                      ecBlocks: [{ numBlocks: 2, dataCodewordsPerBlock: 68 }]
                    },
                    {
                      ecCodewordsPerBlock: 16,
                      ecBlocks: [{ numBlocks: 4, dataCodewordsPerBlock: 27 }]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [{ numBlocks: 4, dataCodewordsPerBlock: 19 }]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [{ numBlocks: 4, dataCodewordsPerBlock: 15 }]
                    }
                  ]
                },
                {
                  infoBits: 31892,
                  versionNumber: 7,
                  alignmentPatternCenters: [6, 22, 38],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 20,
                      ecBlocks: [{ numBlocks: 2, dataCodewordsPerBlock: 78 }]
                    },
                    {
                      ecCodewordsPerBlock: 18,
                      ecBlocks: [{ numBlocks: 4, dataCodewordsPerBlock: 31 }]
                    },
                    {
                      ecCodewordsPerBlock: 18,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 14 },
                        { numBlocks: 4, dataCodewordsPerBlock: 15 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 13 },
                        { numBlocks: 1, dataCodewordsPerBlock: 14 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 34236,
                  versionNumber: 8,
                  alignmentPatternCenters: [6, 24, 42],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [{ numBlocks: 2, dataCodewordsPerBlock: 97 }]
                    },
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 38 },
                        { numBlocks: 2, dataCodewordsPerBlock: 39 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 18 },
                        { numBlocks: 2, dataCodewordsPerBlock: 19 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 14 },
                        { numBlocks: 2, dataCodewordsPerBlock: 15 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 39577,
                  versionNumber: 9,
                  alignmentPatternCenters: [6, 26, 46],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [{ numBlocks: 2, dataCodewordsPerBlock: 116 }]
                    },
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 36 },
                        { numBlocks: 2, dataCodewordsPerBlock: 37 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 20,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 16 },
                        { numBlocks: 4, dataCodewordsPerBlock: 17 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 12 },
                        { numBlocks: 4, dataCodewordsPerBlock: 13 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 42195,
                  versionNumber: 10,
                  alignmentPatternCenters: [6, 28, 50],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 18,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 68 },
                        { numBlocks: 2, dataCodewordsPerBlock: 69 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 43 },
                        { numBlocks: 1, dataCodewordsPerBlock: 44 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 6, dataCodewordsPerBlock: 19 },
                        { numBlocks: 2, dataCodewordsPerBlock: 20 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 6, dataCodewordsPerBlock: 15 },
                        { numBlocks: 2, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 48118,
                  versionNumber: 11,
                  alignmentPatternCenters: [6, 30, 54],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 20,
                      ecBlocks: [{ numBlocks: 4, dataCodewordsPerBlock: 81 }]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 1, dataCodewordsPerBlock: 50 },
                        { numBlocks: 4, dataCodewordsPerBlock: 51 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 22 },
                        { numBlocks: 4, dataCodewordsPerBlock: 23 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 12 },
                        { numBlocks: 8, dataCodewordsPerBlock: 13 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 51042,
                  versionNumber: 12,
                  alignmentPatternCenters: [6, 32, 58],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 92 },
                        { numBlocks: 2, dataCodewordsPerBlock: 93 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [
                        { numBlocks: 6, dataCodewordsPerBlock: 36 },
                        { numBlocks: 2, dataCodewordsPerBlock: 37 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 20 },
                        { numBlocks: 6, dataCodewordsPerBlock: 21 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 7, dataCodewordsPerBlock: 14 },
                        { numBlocks: 4, dataCodewordsPerBlock: 15 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 55367,
                  versionNumber: 13,
                  alignmentPatternCenters: [6, 34, 62],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [{ numBlocks: 4, dataCodewordsPerBlock: 107 }]
                    },
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [
                        { numBlocks: 8, dataCodewordsPerBlock: 37 },
                        { numBlocks: 1, dataCodewordsPerBlock: 38 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 8, dataCodewordsPerBlock: 20 },
                        { numBlocks: 4, dataCodewordsPerBlock: 21 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [
                        { numBlocks: 12, dataCodewordsPerBlock: 11 },
                        { numBlocks: 4, dataCodewordsPerBlock: 12 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 58893,
                  versionNumber: 14,
                  alignmentPatternCenters: [6, 26, 46, 66],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 115 },
                        { numBlocks: 1, dataCodewordsPerBlock: 116 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 40 },
                        { numBlocks: 5, dataCodewordsPerBlock: 41 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 20,
                      ecBlocks: [
                        { numBlocks: 11, dataCodewordsPerBlock: 16 },
                        { numBlocks: 5, dataCodewordsPerBlock: 17 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 11, dataCodewordsPerBlock: 12 },
                        { numBlocks: 5, dataCodewordsPerBlock: 13 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 63784,
                  versionNumber: 15,
                  alignmentPatternCenters: [6, 26, 48, 70],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 22,
                      ecBlocks: [
                        { numBlocks: 5, dataCodewordsPerBlock: 87 },
                        { numBlocks: 1, dataCodewordsPerBlock: 88 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 5, dataCodewordsPerBlock: 41 },
                        { numBlocks: 5, dataCodewordsPerBlock: 42 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 5, dataCodewordsPerBlock: 24 },
                        { numBlocks: 7, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 11, dataCodewordsPerBlock: 12 },
                        { numBlocks: 7, dataCodewordsPerBlock: 13 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 68472,
                  versionNumber: 16,
                  alignmentPatternCenters: [6, 26, 50, 74],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 5, dataCodewordsPerBlock: 98 },
                        { numBlocks: 1, dataCodewordsPerBlock: 99 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 7, dataCodewordsPerBlock: 45 },
                        { numBlocks: 3, dataCodewordsPerBlock: 46 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [
                        { numBlocks: 15, dataCodewordsPerBlock: 19 },
                        { numBlocks: 2, dataCodewordsPerBlock: 20 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 15 },
                        { numBlocks: 13, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 70749,
                  versionNumber: 17,
                  alignmentPatternCenters: [6, 30, 54, 78],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 1, dataCodewordsPerBlock: 107 },
                        { numBlocks: 5, dataCodewordsPerBlock: 108 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 10, dataCodewordsPerBlock: 46 },
                        { numBlocks: 1, dataCodewordsPerBlock: 47 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 1, dataCodewordsPerBlock: 22 },
                        { numBlocks: 15, dataCodewordsPerBlock: 23 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 14 },
                        { numBlocks: 17, dataCodewordsPerBlock: 15 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 76311,
                  versionNumber: 18,
                  alignmentPatternCenters: [6, 30, 56, 82],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 5, dataCodewordsPerBlock: 120 },
                        { numBlocks: 1, dataCodewordsPerBlock: 121 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 9, dataCodewordsPerBlock: 43 },
                        { numBlocks: 4, dataCodewordsPerBlock: 44 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 17, dataCodewordsPerBlock: 22 },
                        { numBlocks: 1, dataCodewordsPerBlock: 23 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 14 },
                        { numBlocks: 19, dataCodewordsPerBlock: 15 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 79154,
                  versionNumber: 19,
                  alignmentPatternCenters: [6, 30, 58, 86],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 113 },
                        { numBlocks: 4, dataCodewordsPerBlock: 114 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 44 },
                        { numBlocks: 11, dataCodewordsPerBlock: 45 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 17, dataCodewordsPerBlock: 21 },
                        { numBlocks: 4, dataCodewordsPerBlock: 22 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 9, dataCodewordsPerBlock: 13 },
                        { numBlocks: 16, dataCodewordsPerBlock: 14 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 84390,
                  versionNumber: 20,
                  alignmentPatternCenters: [6, 34, 62, 90],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 107 },
                        { numBlocks: 5, dataCodewordsPerBlock: 108 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 41 },
                        { numBlocks: 13, dataCodewordsPerBlock: 42 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 15, dataCodewordsPerBlock: 24 },
                        { numBlocks: 5, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 15, dataCodewordsPerBlock: 15 },
                        { numBlocks: 10, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 87683,
                  versionNumber: 21,
                  alignmentPatternCenters: [6, 28, 50, 72, 94],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 116 },
                        { numBlocks: 4, dataCodewordsPerBlock: 117 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [{ numBlocks: 17, dataCodewordsPerBlock: 42 }]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 17, dataCodewordsPerBlock: 22 },
                        { numBlocks: 6, dataCodewordsPerBlock: 23 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 19, dataCodewordsPerBlock: 16 },
                        { numBlocks: 6, dataCodewordsPerBlock: 17 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 92361,
                  versionNumber: 22,
                  alignmentPatternCenters: [6, 26, 50, 74, 98],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 111 },
                        { numBlocks: 7, dataCodewordsPerBlock: 112 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [{ numBlocks: 17, dataCodewordsPerBlock: 46 }]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 7, dataCodewordsPerBlock: 24 },
                        { numBlocks: 16, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 24,
                      ecBlocks: [{ numBlocks: 34, dataCodewordsPerBlock: 13 }]
                    }
                  ]
                },
                {
                  infoBits: 96236,
                  versionNumber: 23,
                  alignmentPatternCenters: [6, 30, 54, 74, 102],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 121 },
                        { numBlocks: 5, dataCodewordsPerBlock: 122 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 47 },
                        { numBlocks: 14, dataCodewordsPerBlock: 48 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 11, dataCodewordsPerBlock: 24 },
                        { numBlocks: 14, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 16, dataCodewordsPerBlock: 15 },
                        { numBlocks: 14, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 102084,
                  versionNumber: 24,
                  alignmentPatternCenters: [6, 28, 54, 80, 106],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 6, dataCodewordsPerBlock: 117 },
                        { numBlocks: 4, dataCodewordsPerBlock: 118 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 6, dataCodewordsPerBlock: 45 },
                        { numBlocks: 14, dataCodewordsPerBlock: 46 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 11, dataCodewordsPerBlock: 24 },
                        { numBlocks: 16, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 30, dataCodewordsPerBlock: 16 },
                        { numBlocks: 2, dataCodewordsPerBlock: 17 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 102881,
                  versionNumber: 25,
                  alignmentPatternCenters: [6, 32, 58, 84, 110],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 26,
                      ecBlocks: [
                        { numBlocks: 8, dataCodewordsPerBlock: 106 },
                        { numBlocks: 4, dataCodewordsPerBlock: 107 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 8, dataCodewordsPerBlock: 47 },
                        { numBlocks: 13, dataCodewordsPerBlock: 48 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 7, dataCodewordsPerBlock: 24 },
                        { numBlocks: 22, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 22, dataCodewordsPerBlock: 15 },
                        { numBlocks: 13, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 110507,
                  versionNumber: 26,
                  alignmentPatternCenters: [6, 30, 58, 86, 114],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 10, dataCodewordsPerBlock: 114 },
                        { numBlocks: 2, dataCodewordsPerBlock: 115 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 19, dataCodewordsPerBlock: 46 },
                        { numBlocks: 4, dataCodewordsPerBlock: 47 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 28, dataCodewordsPerBlock: 22 },
                        { numBlocks: 6, dataCodewordsPerBlock: 23 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 33, dataCodewordsPerBlock: 16 },
                        { numBlocks: 4, dataCodewordsPerBlock: 17 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 110734,
                  versionNumber: 27,
                  alignmentPatternCenters: [6, 34, 62, 90, 118],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 8, dataCodewordsPerBlock: 122 },
                        { numBlocks: 4, dataCodewordsPerBlock: 123 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 22, dataCodewordsPerBlock: 45 },
                        { numBlocks: 3, dataCodewordsPerBlock: 46 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 8, dataCodewordsPerBlock: 23 },
                        { numBlocks: 26, dataCodewordsPerBlock: 24 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 12, dataCodewordsPerBlock: 15 },
                        { numBlocks: 28, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 117786,
                  versionNumber: 28,
                  alignmentPatternCenters: [6, 26, 50, 74, 98, 122],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 117 },
                        { numBlocks: 10, dataCodewordsPerBlock: 118 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 3, dataCodewordsPerBlock: 45 },
                        { numBlocks: 23, dataCodewordsPerBlock: 46 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 24 },
                        { numBlocks: 31, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 11, dataCodewordsPerBlock: 15 },
                        { numBlocks: 31, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 119615,
                  versionNumber: 29,
                  alignmentPatternCenters: [6, 30, 54, 78, 102, 126],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 7, dataCodewordsPerBlock: 116 },
                        { numBlocks: 7, dataCodewordsPerBlock: 117 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 21, dataCodewordsPerBlock: 45 },
                        { numBlocks: 7, dataCodewordsPerBlock: 46 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 1, dataCodewordsPerBlock: 23 },
                        { numBlocks: 37, dataCodewordsPerBlock: 24 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 19, dataCodewordsPerBlock: 15 },
                        { numBlocks: 26, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 126325,
                  versionNumber: 30,
                  alignmentPatternCenters: [6, 26, 52, 78, 104, 130],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 5, dataCodewordsPerBlock: 115 },
                        { numBlocks: 10, dataCodewordsPerBlock: 116 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 19, dataCodewordsPerBlock: 47 },
                        { numBlocks: 10, dataCodewordsPerBlock: 48 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 15, dataCodewordsPerBlock: 24 },
                        { numBlocks: 25, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 23, dataCodewordsPerBlock: 15 },
                        { numBlocks: 25, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 127568,
                  versionNumber: 31,
                  alignmentPatternCenters: [6, 30, 56, 82, 108, 134],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 13, dataCodewordsPerBlock: 115 },
                        { numBlocks: 3, dataCodewordsPerBlock: 116 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 46 },
                        { numBlocks: 29, dataCodewordsPerBlock: 47 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 42, dataCodewordsPerBlock: 24 },
                        { numBlocks: 1, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 23, dataCodewordsPerBlock: 15 },
                        { numBlocks: 28, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 133589,
                  versionNumber: 32,
                  alignmentPatternCenters: [6, 34, 60, 86, 112, 138],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [{ numBlocks: 17, dataCodewordsPerBlock: 115 }]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 10, dataCodewordsPerBlock: 46 },
                        { numBlocks: 23, dataCodewordsPerBlock: 47 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 10, dataCodewordsPerBlock: 24 },
                        { numBlocks: 35, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 19, dataCodewordsPerBlock: 15 },
                        { numBlocks: 35, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 136944,
                  versionNumber: 33,
                  alignmentPatternCenters: [6, 30, 58, 86, 114, 142],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 17, dataCodewordsPerBlock: 115 },
                        { numBlocks: 1, dataCodewordsPerBlock: 116 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 14, dataCodewordsPerBlock: 46 },
                        { numBlocks: 21, dataCodewordsPerBlock: 47 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 29, dataCodewordsPerBlock: 24 },
                        { numBlocks: 19, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 11, dataCodewordsPerBlock: 15 },
                        { numBlocks: 46, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 141498,
                  versionNumber: 34,
                  alignmentPatternCenters: [6, 34, 62, 90, 118, 146],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 13, dataCodewordsPerBlock: 115 },
                        { numBlocks: 6, dataCodewordsPerBlock: 116 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 14, dataCodewordsPerBlock: 46 },
                        { numBlocks: 23, dataCodewordsPerBlock: 47 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 44, dataCodewordsPerBlock: 24 },
                        { numBlocks: 7, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 59, dataCodewordsPerBlock: 16 },
                        { numBlocks: 1, dataCodewordsPerBlock: 17 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 145311,
                  versionNumber: 35,
                  alignmentPatternCenters: [6, 30, 54, 78, 102, 126, 150],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 12, dataCodewordsPerBlock: 121 },
                        { numBlocks: 7, dataCodewordsPerBlock: 122 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 12, dataCodewordsPerBlock: 47 },
                        { numBlocks: 26, dataCodewordsPerBlock: 48 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 39, dataCodewordsPerBlock: 24 },
                        { numBlocks: 14, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 22, dataCodewordsPerBlock: 15 },
                        { numBlocks: 41, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 150283,
                  versionNumber: 36,
                  alignmentPatternCenters: [6, 24, 50, 76, 102, 128, 154],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 6, dataCodewordsPerBlock: 121 },
                        { numBlocks: 14, dataCodewordsPerBlock: 122 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 6, dataCodewordsPerBlock: 47 },
                        { numBlocks: 34, dataCodewordsPerBlock: 48 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 46, dataCodewordsPerBlock: 24 },
                        { numBlocks: 10, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 2, dataCodewordsPerBlock: 15 },
                        { numBlocks: 64, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 152622,
                  versionNumber: 37,
                  alignmentPatternCenters: [6, 28, 54, 80, 106, 132, 158],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 17, dataCodewordsPerBlock: 122 },
                        { numBlocks: 4, dataCodewordsPerBlock: 123 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 29, dataCodewordsPerBlock: 46 },
                        { numBlocks: 14, dataCodewordsPerBlock: 47 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 49, dataCodewordsPerBlock: 24 },
                        { numBlocks: 10, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 24, dataCodewordsPerBlock: 15 },
                        { numBlocks: 46, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 158308,
                  versionNumber: 38,
                  alignmentPatternCenters: [6, 32, 58, 84, 110, 136, 162],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 4, dataCodewordsPerBlock: 122 },
                        { numBlocks: 18, dataCodewordsPerBlock: 123 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 13, dataCodewordsPerBlock: 46 },
                        { numBlocks: 32, dataCodewordsPerBlock: 47 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 48, dataCodewordsPerBlock: 24 },
                        { numBlocks: 14, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 42, dataCodewordsPerBlock: 15 },
                        { numBlocks: 32, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 161089,
                  versionNumber: 39,
                  alignmentPatternCenters: [6, 26, 54, 82, 110, 138, 166],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 20, dataCodewordsPerBlock: 117 },
                        { numBlocks: 4, dataCodewordsPerBlock: 118 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 40, dataCodewordsPerBlock: 47 },
                        { numBlocks: 7, dataCodewordsPerBlock: 48 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 43, dataCodewordsPerBlock: 24 },
                        { numBlocks: 22, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 10, dataCodewordsPerBlock: 15 },
                        { numBlocks: 67, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                },
                {
                  infoBits: 167017,
                  versionNumber: 40,
                  alignmentPatternCenters: [6, 30, 58, 86, 114, 142, 170],
                  errorCorrectionLevels: [
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 19, dataCodewordsPerBlock: 118 },
                        { numBlocks: 6, dataCodewordsPerBlock: 119 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 28,
                      ecBlocks: [
                        { numBlocks: 18, dataCodewordsPerBlock: 47 },
                        { numBlocks: 31, dataCodewordsPerBlock: 48 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 34, dataCodewordsPerBlock: 24 },
                        { numBlocks: 34, dataCodewordsPerBlock: 25 }
                      ]
                    },
                    {
                      ecCodewordsPerBlock: 30,
                      ecBlocks: [
                        { numBlocks: 20, dataCodewordsPerBlock: 15 },
                        { numBlocks: 61, dataCodewordsPerBlock: 16 }
                      ]
                    }
                  ]
                }
              ];
            }),
            /* 11 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var BitMatrix_1 = __webpack_require__(0);
              function squareToQuadrilateral(p1, p2, p3, p4) {
                var dx3 = p1.x - p2.x + p3.x - p4.x, dy3 = p1.y - p2.y + p3.y - p4.y;
                if (dx3 === 0 && dy3 === 0)
                  return {
                    a11: p2.x - p1.x,
                    a12: p2.y - p1.y,
                    a13: 0,
                    a21: p3.x - p2.x,
                    a22: p3.y - p2.y,
                    a23: 0,
                    a31: p1.x,
                    a32: p1.y,
                    a33: 1
                  };
                var dx1 = p2.x - p3.x, dx2 = p4.x - p3.x, dy1 = p2.y - p3.y, dy2 = p4.y - p3.y, denominator = dx1 * dy2 - dx2 * dy1, a13 = (dx3 * dy2 - dx2 * dy3) / denominator, a23 = (dx1 * dy3 - dx3 * dy1) / denominator;
                return {
                  a11: p2.x - p1.x + a13 * p2.x,
                  a12: p2.y - p1.y + a13 * p2.y,
                  a13,
                  a21: p4.x - p1.x + a23 * p4.x,
                  a22: p4.y - p1.y + a23 * p4.y,
                  a23,
                  a31: p1.x,
                  a32: p1.y,
                  a33: 1
                };
              }
              function quadrilateralToSquare(p1, p2, p3, p4) {
                var sToQ = squareToQuadrilateral(p1, p2, p3, p4);
                return {
                  a11: sToQ.a22 * sToQ.a33 - sToQ.a23 * sToQ.a32,
                  a12: sToQ.a13 * sToQ.a32 - sToQ.a12 * sToQ.a33,
                  a13: sToQ.a12 * sToQ.a23 - sToQ.a13 * sToQ.a22,
                  a21: sToQ.a23 * sToQ.a31 - sToQ.a21 * sToQ.a33,
                  a22: sToQ.a11 * sToQ.a33 - sToQ.a13 * sToQ.a31,
                  a23: sToQ.a13 * sToQ.a21 - sToQ.a11 * sToQ.a23,
                  a31: sToQ.a21 * sToQ.a32 - sToQ.a22 * sToQ.a31,
                  a32: sToQ.a12 * sToQ.a31 - sToQ.a11 * sToQ.a32,
                  a33: sToQ.a11 * sToQ.a22 - sToQ.a12 * sToQ.a21
                };
              }
              function times(a, b) {
                return {
                  a11: a.a11 * b.a11 + a.a21 * b.a12 + a.a31 * b.a13,
                  a12: a.a12 * b.a11 + a.a22 * b.a12 + a.a32 * b.a13,
                  a13: a.a13 * b.a11 + a.a23 * b.a12 + a.a33 * b.a13,
                  a21: a.a11 * b.a21 + a.a21 * b.a22 + a.a31 * b.a23,
                  a22: a.a12 * b.a21 + a.a22 * b.a22 + a.a32 * b.a23,
                  a23: a.a13 * b.a21 + a.a23 * b.a22 + a.a33 * b.a23,
                  a31: a.a11 * b.a31 + a.a21 * b.a32 + a.a31 * b.a33,
                  a32: a.a12 * b.a31 + a.a22 * b.a32 + a.a32 * b.a33,
                  a33: a.a13 * b.a31 + a.a23 * b.a32 + a.a33 * b.a33
                };
              }
              function extract(image, location) {
                for (var qToS = quadrilateralToSquare({ x: 3.5, y: 3.5 }, { x: location.dimension - 3.5, y: 3.5 }, { x: location.dimension - 6.5, y: location.dimension - 6.5 }, { x: 3.5, y: location.dimension - 3.5 }), sToQ = squareToQuadrilateral(location.topLeft, location.topRight, location.alignmentPattern, location.bottomLeft), transform = times(sToQ, qToS), matrix = BitMatrix_1.BitMatrix.createEmpty(location.dimension, location.dimension), mappingFunction = function(x2, y2) {
                  var denominator = transform.a13 * x2 + transform.a23 * y2 + transform.a33;
                  return {
                    x: (transform.a11 * x2 + transform.a21 * y2 + transform.a31) / denominator,
                    y: (transform.a12 * x2 + transform.a22 * y2 + transform.a32) / denominator
                  };
                }, y = 0; y < location.dimension; y++)
                  for (var x = 0; x < location.dimension; x++) {
                    var xValue = x + 0.5, yValue = y + 0.5, sourcePixel = mappingFunction(xValue, yValue);
                    matrix.set(x, y, image.get(Math.floor(sourcePixel.x), Math.floor(sourcePixel.y)));
                  }
                return {
                  matrix,
                  mappingFunction
                };
              }
              exports2.extract = extract;
            }),
            /* 12 */
            /***/
            (function(module2, exports2, __webpack_require__) {
              "use strict";
              Object.defineProperty(exports2, "__esModule", { value: !0 });
              var MAX_FINDERPATTERNS_TO_SEARCH = 4, MIN_QUAD_RATIO = 0.5, MAX_QUAD_RATIO = 1.5, distance = function(a, b) {
                return Math.sqrt(Math.pow(b.x - a.x, 2) + Math.pow(b.y - a.y, 2));
              };
              function sum(values) {
                return values.reduce(function(a, b) {
                  return a + b;
                });
              }
              function reorderFinderPatterns(pattern1, pattern2, pattern3) {
                var _a, _b, _c, _d, oneTwoDistance = distance(pattern1, pattern2), twoThreeDistance = distance(pattern2, pattern3), oneThreeDistance = distance(pattern1, pattern3), bottomLeft, topLeft, topRight;
                return twoThreeDistance >= oneTwoDistance && twoThreeDistance >= oneThreeDistance ? (_a = [pattern2, pattern1, pattern3], bottomLeft = _a[0], topLeft = _a[1], topRight = _a[2]) : oneThreeDistance >= twoThreeDistance && oneThreeDistance >= oneTwoDistance ? (_b = [pattern1, pattern2, pattern3], bottomLeft = _b[0], topLeft = _b[1], topRight = _b[2]) : (_c = [pattern1, pattern3, pattern2], bottomLeft = _c[0], topLeft = _c[1], topRight = _c[2]), (topRight.x - topLeft.x) * (bottomLeft.y - topLeft.y) - (topRight.y - topLeft.y) * (bottomLeft.x - topLeft.x) < 0 && (_d = [topRight, bottomLeft], bottomLeft = _d[0], topRight = _d[1]), { bottomLeft, topLeft, topRight };
              }
              function computeDimension(topLeft, topRight, bottomLeft, matrix) {
                var moduleSize = (sum(countBlackWhiteRun(topLeft, bottomLeft, matrix, 5)) / 7 + // Divide by 7 since the ratio is 1:1:3:1:1
                sum(countBlackWhiteRun(topLeft, topRight, matrix, 5)) / 7 + sum(countBlackWhiteRun(bottomLeft, topLeft, matrix, 5)) / 7 + sum(countBlackWhiteRun(topRight, topLeft, matrix, 5)) / 7) / 4;
                if (moduleSize < 1)
                  throw new Error("Invalid module size");
                var topDimension = Math.round(distance(topLeft, topRight) / moduleSize), sideDimension = Math.round(distance(topLeft, bottomLeft) / moduleSize), dimension = Math.floor((topDimension + sideDimension) / 2) + 7;
                switch (dimension % 4) {
                  case 0:
                    dimension++;
                    break;
                  case 2:
                    dimension--;
                    break;
                }
                return { dimension, moduleSize };
              }
              function countBlackWhiteRunTowardsPoint(origin, end, matrix, length) {
                var switchPoints = [{ x: Math.floor(origin.x), y: Math.floor(origin.y) }], steep = Math.abs(end.y - origin.y) > Math.abs(end.x - origin.x), fromX, fromY, toX, toY;
                steep ? (fromX = Math.floor(origin.y), fromY = Math.floor(origin.x), toX = Math.floor(end.y), toY = Math.floor(end.x)) : (fromX = Math.floor(origin.x), fromY = Math.floor(origin.y), toX = Math.floor(end.x), toY = Math.floor(end.y));
                for (var dx = Math.abs(toX - fromX), dy = Math.abs(toY - fromY), error = Math.floor(-dx / 2), xStep = fromX < toX ? 1 : -1, yStep = fromY < toY ? 1 : -1, currentPixel = !0, x = fromX, y = fromY; x !== toX + xStep; x += xStep) {
                  var realX = steep ? y : x, realY = steep ? x : y;
                  if (matrix.get(realX, realY) !== currentPixel && (currentPixel = !currentPixel, switchPoints.push({ x: realX, y: realY }), switchPoints.length === length + 1))
                    break;
                  if (error += dy, error > 0) {
                    if (y === toY)
                      break;
                    y += yStep, error -= dx;
                  }
                }
                for (var distances = [], i = 0; i < length; i++)
                  switchPoints[i] && switchPoints[i + 1] ? distances.push(distance(switchPoints[i], switchPoints[i + 1])) : distances.push(0);
                return distances;
              }
              function countBlackWhiteRun(origin, end, matrix, length) {
                var _a, rise = end.y - origin.y, run = end.x - origin.x, towardsEnd = countBlackWhiteRunTowardsPoint(origin, end, matrix, Math.ceil(length / 2)), awayFromEnd = countBlackWhiteRunTowardsPoint(origin, { x: origin.x - run, y: origin.y - rise }, matrix, Math.ceil(length / 2)), middleValue = towardsEnd.shift() + awayFromEnd.shift() - 1;
                return (_a = awayFromEnd.concat(middleValue)).concat.apply(_a, towardsEnd);
              }
              function scoreBlackWhiteRun(sequence, ratios) {
                var averageSize = sum(sequence) / sum(ratios), error = 0;
                return ratios.forEach(function(ratio, i) {
                  error += Math.pow(sequence[i] - ratio * averageSize, 2);
                }), { averageSize, error };
              }
              function scorePattern(point, ratios, matrix) {
                try {
                  var horizontalRun = countBlackWhiteRun(point, { x: -1, y: point.y }, matrix, ratios.length), verticalRun = countBlackWhiteRun(point, { x: point.x, y: -1 }, matrix, ratios.length), topLeftPoint = {
                    x: Math.max(0, point.x - point.y) - 1,
                    y: Math.max(0, point.y - point.x) - 1
                  }, topLeftBottomRightRun = countBlackWhiteRun(point, topLeftPoint, matrix, ratios.length), bottomLeftPoint = {
                    x: Math.min(matrix.width, point.x + point.y) + 1,
                    y: Math.min(matrix.height, point.y + point.x) + 1
                  }, bottomLeftTopRightRun = countBlackWhiteRun(point, bottomLeftPoint, matrix, ratios.length), horzError = scoreBlackWhiteRun(horizontalRun, ratios), vertError = scoreBlackWhiteRun(verticalRun, ratios), diagDownError = scoreBlackWhiteRun(topLeftBottomRightRun, ratios), diagUpError = scoreBlackWhiteRun(bottomLeftTopRightRun, ratios), ratioError = Math.sqrt(horzError.error * horzError.error + vertError.error * vertError.error + diagDownError.error * diagDownError.error + diagUpError.error * diagUpError.error), avgSize = (horzError.averageSize + vertError.averageSize + diagDownError.averageSize + diagUpError.averageSize) / 4, sizeError = (Math.pow(horzError.averageSize - avgSize, 2) + Math.pow(vertError.averageSize - avgSize, 2) + Math.pow(diagDownError.averageSize - avgSize, 2) + Math.pow(diagUpError.averageSize - avgSize, 2)) / avgSize;
                  return ratioError + sizeError;
                } catch (_a) {
                  return 1 / 0;
                }
              }
              function recenterLocation(matrix, p) {
                for (var leftX = Math.round(p.x); matrix.get(leftX, Math.round(p.y)); )
                  leftX--;
                for (var rightX = Math.round(p.x); matrix.get(rightX, Math.round(p.y)); )
                  rightX++;
                for (var x = (leftX + rightX) / 2, topY = Math.round(p.y); matrix.get(Math.round(x), topY); )
                  topY--;
                for (var bottomY = Math.round(p.y); matrix.get(Math.round(x), bottomY); )
                  bottomY++;
                var y = (topY + bottomY) / 2;
                return { x, y };
              }
              function locate(matrix) {
                for (var finderPatternQuads = [], activeFinderPatternQuads = [], alignmentPatternQuads = [], activeAlignmentPatternQuads = [], _loop_1 = function(y2) {
                  for (var length_1 = 0, lastBit = !1, scans = [0, 0, 0, 0, 0], _loop_2 = function(x2) {
                    var v = matrix.get(x2, y2);
                    if (v === lastBit)
                      length_1++;
                    else {
                      scans = [scans[1], scans[2], scans[3], scans[4], length_1], length_1 = 1, lastBit = v;
                      var averageFinderPatternBlocksize = sum(scans) / 7, validFinderPattern = Math.abs(scans[0] - averageFinderPatternBlocksize) < averageFinderPatternBlocksize && Math.abs(scans[1] - averageFinderPatternBlocksize) < averageFinderPatternBlocksize && Math.abs(scans[2] - 3 * averageFinderPatternBlocksize) < 3 * averageFinderPatternBlocksize && Math.abs(scans[3] - averageFinderPatternBlocksize) < averageFinderPatternBlocksize && Math.abs(scans[4] - averageFinderPatternBlocksize) < averageFinderPatternBlocksize && !v, averageAlignmentPatternBlocksize = sum(scans.slice(-3)) / 3, validAlignmentPattern = Math.abs(scans[2] - averageAlignmentPatternBlocksize) < averageAlignmentPatternBlocksize && Math.abs(scans[3] - averageAlignmentPatternBlocksize) < averageAlignmentPatternBlocksize && Math.abs(scans[4] - averageAlignmentPatternBlocksize) < averageAlignmentPatternBlocksize && v;
                      if (validFinderPattern) {
                        var endX_1 = x2 - scans[3] - scans[4], startX_1 = endX_1 - scans[2], line = { startX: startX_1, endX: endX_1, y: y2 }, matchingQuads = activeFinderPatternQuads.filter(function(q) {
                          return startX_1 >= q.bottom.startX && startX_1 <= q.bottom.endX || endX_1 >= q.bottom.startX && startX_1 <= q.bottom.endX || startX_1 <= q.bottom.startX && endX_1 >= q.bottom.endX && scans[2] / (q.bottom.endX - q.bottom.startX) < MAX_QUAD_RATIO && scans[2] / (q.bottom.endX - q.bottom.startX) > MIN_QUAD_RATIO;
                        });
                        matchingQuads.length > 0 ? matchingQuads[0].bottom = line : activeFinderPatternQuads.push({ top: line, bottom: line });
                      }
                      if (validAlignmentPattern) {
                        var endX_2 = x2 - scans[4], startX_2 = endX_2 - scans[3], line = { startX: startX_2, y: y2, endX: endX_2 }, matchingQuads = activeAlignmentPatternQuads.filter(function(q) {
                          return startX_2 >= q.bottom.startX && startX_2 <= q.bottom.endX || endX_2 >= q.bottom.startX && startX_2 <= q.bottom.endX || startX_2 <= q.bottom.startX && endX_2 >= q.bottom.endX && scans[2] / (q.bottom.endX - q.bottom.startX) < MAX_QUAD_RATIO && scans[2] / (q.bottom.endX - q.bottom.startX) > MIN_QUAD_RATIO;
                        });
                        matchingQuads.length > 0 ? matchingQuads[0].bottom = line : activeAlignmentPatternQuads.push({ top: line, bottom: line });
                      }
                    }
                  }, x = -1; x <= matrix.width; x++)
                    _loop_2(x);
                  finderPatternQuads.push.apply(finderPatternQuads, activeFinderPatternQuads.filter(function(q) {
                    return q.bottom.y !== y2 && q.bottom.y - q.top.y >= 2;
                  })), activeFinderPatternQuads = activeFinderPatternQuads.filter(function(q) {
                    return q.bottom.y === y2;
                  }), alignmentPatternQuads.push.apply(alignmentPatternQuads, activeAlignmentPatternQuads.filter(function(q) {
                    return q.bottom.y !== y2;
                  })), activeAlignmentPatternQuads = activeAlignmentPatternQuads.filter(function(q) {
                    return q.bottom.y === y2;
                  });
                }, y = 0; y <= matrix.height; y++)
                  _loop_1(y);
                finderPatternQuads.push.apply(finderPatternQuads, activeFinderPatternQuads.filter(function(q) {
                  return q.bottom.y - q.top.y >= 2;
                })), alignmentPatternQuads.push.apply(alignmentPatternQuads, activeAlignmentPatternQuads);
                var finderPatternGroups = finderPatternQuads.filter(function(q) {
                  return q.bottom.y - q.top.y >= 2;
                }).map(function(q) {
                  var x = (q.top.startX + q.top.endX + q.bottom.startX + q.bottom.endX) / 4, y2 = (q.top.y + q.bottom.y + 1) / 2;
                  if (matrix.get(Math.round(x), Math.round(y2))) {
                    var lengths = [q.top.endX - q.top.startX, q.bottom.endX - q.bottom.startX, q.bottom.y - q.top.y + 1], size = sum(lengths) / lengths.length, score = scorePattern({ x: Math.round(x), y: Math.round(y2) }, [1, 1, 3, 1, 1], matrix);
                    return { score, x, y: y2, size };
                  }
                }).filter(function(q) {
                  return !!q;
                }).sort(function(a, b) {
                  return a.score - b.score;
                }).map(function(point, i, finderPatterns) {
                  if (i > MAX_FINDERPATTERNS_TO_SEARCH)
                    return null;
                  var otherPoints = finderPatterns.filter(function(p, ii) {
                    return i !== ii;
                  }).map(function(p) {
                    return { x: p.x, y: p.y, score: p.score + Math.pow(p.size - point.size, 2) / point.size, size: p.size };
                  }).sort(function(a, b) {
                    return a.score - b.score;
                  });
                  if (otherPoints.length < 2)
                    return null;
                  var score = point.score + otherPoints[0].score + otherPoints[1].score;
                  return { points: [point].concat(otherPoints.slice(0, 2)), score };
                }).filter(function(q) {
                  return !!q;
                }).sort(function(a, b) {
                  return a.score - b.score;
                });
                if (finderPatternGroups.length === 0)
                  return null;
                var _a = reorderFinderPatterns(finderPatternGroups[0].points[0], finderPatternGroups[0].points[1], finderPatternGroups[0].points[2]), topRight = _a.topRight, topLeft = _a.topLeft, bottomLeft = _a.bottomLeft, alignment = findAlignmentPattern(matrix, alignmentPatternQuads, topRight, topLeft, bottomLeft), result = [];
                alignment && result.push({
                  alignmentPattern: { x: alignment.alignmentPattern.x, y: alignment.alignmentPattern.y },
                  bottomLeft: { x: bottomLeft.x, y: bottomLeft.y },
                  dimension: alignment.dimension,
                  topLeft: { x: topLeft.x, y: topLeft.y },
                  topRight: { x: topRight.x, y: topRight.y }
                });
                var midTopRight = recenterLocation(matrix, topRight), midTopLeft = recenterLocation(matrix, topLeft), midBottomLeft = recenterLocation(matrix, bottomLeft), centeredAlignment = findAlignmentPattern(matrix, alignmentPatternQuads, midTopRight, midTopLeft, midBottomLeft);
                return centeredAlignment && result.push({
                  alignmentPattern: { x: centeredAlignment.alignmentPattern.x, y: centeredAlignment.alignmentPattern.y },
                  bottomLeft: { x: midBottomLeft.x, y: midBottomLeft.y },
                  topLeft: { x: midTopLeft.x, y: midTopLeft.y },
                  topRight: { x: midTopRight.x, y: midTopRight.y },
                  dimension: centeredAlignment.dimension
                }), result.length === 0 ? null : result;
              }
              exports2.locate = locate;
              function findAlignmentPattern(matrix, alignmentPatternQuads, topRight, topLeft, bottomLeft) {
                var _a, dimension, moduleSize;
                try {
                  _a = computeDimension(topLeft, topRight, bottomLeft, matrix), dimension = _a.dimension, moduleSize = _a.moduleSize;
                } catch (e) {
                  return null;
                }
                var bottomRightFinderPattern = {
                  x: topRight.x - topLeft.x + bottomLeft.x,
                  y: topRight.y - topLeft.y + bottomLeft.y
                }, modulesBetweenFinderPatterns = (distance(topLeft, bottomLeft) + distance(topLeft, topRight)) / 2 / moduleSize, correctionToTopLeft = 1 - 3 / modulesBetweenFinderPatterns, expectedAlignmentPattern = {
                  x: topLeft.x + correctionToTopLeft * (bottomRightFinderPattern.x - topLeft.x),
                  y: topLeft.y + correctionToTopLeft * (bottomRightFinderPattern.y - topLeft.y)
                }, alignmentPatterns = alignmentPatternQuads.map(function(q) {
                  var x = (q.top.startX + q.top.endX + q.bottom.startX + q.bottom.endX) / 4, y = (q.top.y + q.bottom.y + 1) / 2;
                  if (matrix.get(Math.floor(x), Math.floor(y))) {
                    var lengths = [q.top.endX - q.top.startX, q.bottom.endX - q.bottom.startX, q.bottom.y - q.top.y + 1], size = sum(lengths) / lengths.length, sizeScore = scorePattern({ x: Math.floor(x), y: Math.floor(y) }, [1, 1, 1], matrix), score = sizeScore + distance({ x, y }, expectedAlignmentPattern);
                    return { x, y, score };
                  }
                }).filter(function(v) {
                  return !!v;
                }).sort(function(a, b) {
                  return a.score - b.score;
                }), alignmentPattern = modulesBetweenFinderPatterns >= 15 && alignmentPatterns.length ? alignmentPatterns[0] : expectedAlignmentPattern;
                return { alignmentPattern, dimension };
              }
            })
            /******/
          ]).default
        );
      });
    }
  });

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

  // ../../node_modules/.pnpm/tweetnacl@1.0.3/node_modules/tweetnacl/nacl-fast.js
  var require_nacl_fast = __commonJS({
    "../../node_modules/.pnpm/tweetnacl@1.0.3/node_modules/tweetnacl/nacl-fast.js"(exports, module) {
      (function(nacl2) {
        "use strict";
        var gf = function(init) {
          var i, r = new Float64Array(16);
          if (init) for (i = 0; i < init.length; i++) r[i] = init[i];
          return r;
        }, randombytes = function() {
          throw new Error("no PRNG");
        }, _0 = new Uint8Array(16), _9 = new Uint8Array(32);
        _9[0] = 9;
        var gf0 = gf(), gf1 = gf([1]), _121665 = gf([56129, 1]), D = gf([30883, 4953, 19914, 30187, 55467, 16705, 2637, 112, 59544, 30585, 16505, 36039, 65139, 11119, 27886, 20995]), D2 = gf([61785, 9906, 39828, 60374, 45398, 33411, 5274, 224, 53552, 61171, 33010, 6542, 64743, 22239, 55772, 9222]), X = gf([54554, 36645, 11616, 51542, 42930, 38181, 51040, 26924, 56412, 64982, 57905, 49316, 21502, 52590, 14035, 8553]), Y = gf([26200, 26214, 26214, 26214, 26214, 26214, 26214, 26214, 26214, 26214, 26214, 26214, 26214, 26214, 26214, 26214]), I = gf([41136, 18958, 6951, 50414, 58488, 44335, 6150, 12099, 55207, 15867, 153, 11085, 57099, 20417, 9344, 11139]);
        function ts64(x, i, h, l) {
          x[i] = h >> 24 & 255, x[i + 1] = h >> 16 & 255, x[i + 2] = h >> 8 & 255, x[i + 3] = h & 255, x[i + 4] = l >> 24 & 255, x[i + 5] = l >> 16 & 255, x[i + 6] = l >> 8 & 255, x[i + 7] = l & 255;
        }
        function vn(x, xi, y, yi, n) {
          var i, d = 0;
          for (i = 0; i < n; i++) d |= x[xi + i] ^ y[yi + i];
          return (1 & d - 1 >>> 8) - 1;
        }
        function crypto_verify_16(x, xi, y, yi) {
          return vn(x, xi, y, yi, 16);
        }
        function crypto_verify_32(x, xi, y, yi) {
          return vn(x, xi, y, yi, 32);
        }
        function core_salsa20(o, p, k, c) {
          for (var j0 = c[0] & 255 | (c[1] & 255) << 8 | (c[2] & 255) << 16 | (c[3] & 255) << 24, j1 = k[0] & 255 | (k[1] & 255) << 8 | (k[2] & 255) << 16 | (k[3] & 255) << 24, j2 = k[4] & 255 | (k[5] & 255) << 8 | (k[6] & 255) << 16 | (k[7] & 255) << 24, j3 = k[8] & 255 | (k[9] & 255) << 8 | (k[10] & 255) << 16 | (k[11] & 255) << 24, j4 = k[12] & 255 | (k[13] & 255) << 8 | (k[14] & 255) << 16 | (k[15] & 255) << 24, j5 = c[4] & 255 | (c[5] & 255) << 8 | (c[6] & 255) << 16 | (c[7] & 255) << 24, j6 = p[0] & 255 | (p[1] & 255) << 8 | (p[2] & 255) << 16 | (p[3] & 255) << 24, j7 = p[4] & 255 | (p[5] & 255) << 8 | (p[6] & 255) << 16 | (p[7] & 255) << 24, j8 = p[8] & 255 | (p[9] & 255) << 8 | (p[10] & 255) << 16 | (p[11] & 255) << 24, j9 = p[12] & 255 | (p[13] & 255) << 8 | (p[14] & 255) << 16 | (p[15] & 255) << 24, j10 = c[8] & 255 | (c[9] & 255) << 8 | (c[10] & 255) << 16 | (c[11] & 255) << 24, j11 = k[16] & 255 | (k[17] & 255) << 8 | (k[18] & 255) << 16 | (k[19] & 255) << 24, j12 = k[20] & 255 | (k[21] & 255) << 8 | (k[22] & 255) << 16 | (k[23] & 255) << 24, j13 = k[24] & 255 | (k[25] & 255) << 8 | (k[26] & 255) << 16 | (k[27] & 255) << 24, j14 = k[28] & 255 | (k[29] & 255) << 8 | (k[30] & 255) << 16 | (k[31] & 255) << 24, j15 = c[12] & 255 | (c[13] & 255) << 8 | (c[14] & 255) << 16 | (c[15] & 255) << 24, x0 = j0, x1 = j1, x2 = j2, x3 = j3, x4 = j4, x5 = j5, x6 = j6, x7 = j7, x8 = j8, x9 = j9, x10 = j10, x11 = j11, x12 = j12, x13 = j13, x14 = j14, x15 = j15, u, i = 0; i < 20; i += 2)
            u = x0 + x12 | 0, x4 ^= u << 7 | u >>> 25, u = x4 + x0 | 0, x8 ^= u << 9 | u >>> 23, u = x8 + x4 | 0, x12 ^= u << 13 | u >>> 19, u = x12 + x8 | 0, x0 ^= u << 18 | u >>> 14, u = x5 + x1 | 0, x9 ^= u << 7 | u >>> 25, u = x9 + x5 | 0, x13 ^= u << 9 | u >>> 23, u = x13 + x9 | 0, x1 ^= u << 13 | u >>> 19, u = x1 + x13 | 0, x5 ^= u << 18 | u >>> 14, u = x10 + x6 | 0, x14 ^= u << 7 | u >>> 25, u = x14 + x10 | 0, x2 ^= u << 9 | u >>> 23, u = x2 + x14 | 0, x6 ^= u << 13 | u >>> 19, u = x6 + x2 | 0, x10 ^= u << 18 | u >>> 14, u = x15 + x11 | 0, x3 ^= u << 7 | u >>> 25, u = x3 + x15 | 0, x7 ^= u << 9 | u >>> 23, u = x7 + x3 | 0, x11 ^= u << 13 | u >>> 19, u = x11 + x7 | 0, x15 ^= u << 18 | u >>> 14, u = x0 + x3 | 0, x1 ^= u << 7 | u >>> 25, u = x1 + x0 | 0, x2 ^= u << 9 | u >>> 23, u = x2 + x1 | 0, x3 ^= u << 13 | u >>> 19, u = x3 + x2 | 0, x0 ^= u << 18 | u >>> 14, u = x5 + x4 | 0, x6 ^= u << 7 | u >>> 25, u = x6 + x5 | 0, x7 ^= u << 9 | u >>> 23, u = x7 + x6 | 0, x4 ^= u << 13 | u >>> 19, u = x4 + x7 | 0, x5 ^= u << 18 | u >>> 14, u = x10 + x9 | 0, x11 ^= u << 7 | u >>> 25, u = x11 + x10 | 0, x8 ^= u << 9 | u >>> 23, u = x8 + x11 | 0, x9 ^= u << 13 | u >>> 19, u = x9 + x8 | 0, x10 ^= u << 18 | u >>> 14, u = x15 + x14 | 0, x12 ^= u << 7 | u >>> 25, u = x12 + x15 | 0, x13 ^= u << 9 | u >>> 23, u = x13 + x12 | 0, x14 ^= u << 13 | u >>> 19, u = x14 + x13 | 0, x15 ^= u << 18 | u >>> 14;
          x0 = x0 + j0 | 0, x1 = x1 + j1 | 0, x2 = x2 + j2 | 0, x3 = x3 + j3 | 0, x4 = x4 + j4 | 0, x5 = x5 + j5 | 0, x6 = x6 + j6 | 0, x7 = x7 + j7 | 0, x8 = x8 + j8 | 0, x9 = x9 + j9 | 0, x10 = x10 + j10 | 0, x11 = x11 + j11 | 0, x12 = x12 + j12 | 0, x13 = x13 + j13 | 0, x14 = x14 + j14 | 0, x15 = x15 + j15 | 0, o[0] = x0 >>> 0 & 255, o[1] = x0 >>> 8 & 255, o[2] = x0 >>> 16 & 255, o[3] = x0 >>> 24 & 255, o[4] = x1 >>> 0 & 255, o[5] = x1 >>> 8 & 255, o[6] = x1 >>> 16 & 255, o[7] = x1 >>> 24 & 255, o[8] = x2 >>> 0 & 255, o[9] = x2 >>> 8 & 255, o[10] = x2 >>> 16 & 255, o[11] = x2 >>> 24 & 255, o[12] = x3 >>> 0 & 255, o[13] = x3 >>> 8 & 255, o[14] = x3 >>> 16 & 255, o[15] = x3 >>> 24 & 255, o[16] = x4 >>> 0 & 255, o[17] = x4 >>> 8 & 255, o[18] = x4 >>> 16 & 255, o[19] = x4 >>> 24 & 255, o[20] = x5 >>> 0 & 255, o[21] = x5 >>> 8 & 255, o[22] = x5 >>> 16 & 255, o[23] = x5 >>> 24 & 255, o[24] = x6 >>> 0 & 255, o[25] = x6 >>> 8 & 255, o[26] = x6 >>> 16 & 255, o[27] = x6 >>> 24 & 255, o[28] = x7 >>> 0 & 255, o[29] = x7 >>> 8 & 255, o[30] = x7 >>> 16 & 255, o[31] = x7 >>> 24 & 255, o[32] = x8 >>> 0 & 255, o[33] = x8 >>> 8 & 255, o[34] = x8 >>> 16 & 255, o[35] = x8 >>> 24 & 255, o[36] = x9 >>> 0 & 255, o[37] = x9 >>> 8 & 255, o[38] = x9 >>> 16 & 255, o[39] = x9 >>> 24 & 255, o[40] = x10 >>> 0 & 255, o[41] = x10 >>> 8 & 255, o[42] = x10 >>> 16 & 255, o[43] = x10 >>> 24 & 255, o[44] = x11 >>> 0 & 255, o[45] = x11 >>> 8 & 255, o[46] = x11 >>> 16 & 255, o[47] = x11 >>> 24 & 255, o[48] = x12 >>> 0 & 255, o[49] = x12 >>> 8 & 255, o[50] = x12 >>> 16 & 255, o[51] = x12 >>> 24 & 255, o[52] = x13 >>> 0 & 255, o[53] = x13 >>> 8 & 255, o[54] = x13 >>> 16 & 255, o[55] = x13 >>> 24 & 255, o[56] = x14 >>> 0 & 255, o[57] = x14 >>> 8 & 255, o[58] = x14 >>> 16 & 255, o[59] = x14 >>> 24 & 255, o[60] = x15 >>> 0 & 255, o[61] = x15 >>> 8 & 255, o[62] = x15 >>> 16 & 255, o[63] = x15 >>> 24 & 255;
        }
        function core_hsalsa20(o, p, k, c) {
          for (var j0 = c[0] & 255 | (c[1] & 255) << 8 | (c[2] & 255) << 16 | (c[3] & 255) << 24, j1 = k[0] & 255 | (k[1] & 255) << 8 | (k[2] & 255) << 16 | (k[3] & 255) << 24, j2 = k[4] & 255 | (k[5] & 255) << 8 | (k[6] & 255) << 16 | (k[7] & 255) << 24, j3 = k[8] & 255 | (k[9] & 255) << 8 | (k[10] & 255) << 16 | (k[11] & 255) << 24, j4 = k[12] & 255 | (k[13] & 255) << 8 | (k[14] & 255) << 16 | (k[15] & 255) << 24, j5 = c[4] & 255 | (c[5] & 255) << 8 | (c[6] & 255) << 16 | (c[7] & 255) << 24, j6 = p[0] & 255 | (p[1] & 255) << 8 | (p[2] & 255) << 16 | (p[3] & 255) << 24, j7 = p[4] & 255 | (p[5] & 255) << 8 | (p[6] & 255) << 16 | (p[7] & 255) << 24, j8 = p[8] & 255 | (p[9] & 255) << 8 | (p[10] & 255) << 16 | (p[11] & 255) << 24, j9 = p[12] & 255 | (p[13] & 255) << 8 | (p[14] & 255) << 16 | (p[15] & 255) << 24, j10 = c[8] & 255 | (c[9] & 255) << 8 | (c[10] & 255) << 16 | (c[11] & 255) << 24, j11 = k[16] & 255 | (k[17] & 255) << 8 | (k[18] & 255) << 16 | (k[19] & 255) << 24, j12 = k[20] & 255 | (k[21] & 255) << 8 | (k[22] & 255) << 16 | (k[23] & 255) << 24, j13 = k[24] & 255 | (k[25] & 255) << 8 | (k[26] & 255) << 16 | (k[27] & 255) << 24, j14 = k[28] & 255 | (k[29] & 255) << 8 | (k[30] & 255) << 16 | (k[31] & 255) << 24, j15 = c[12] & 255 | (c[13] & 255) << 8 | (c[14] & 255) << 16 | (c[15] & 255) << 24, x0 = j0, x1 = j1, x2 = j2, x3 = j3, x4 = j4, x5 = j5, x6 = j6, x7 = j7, x8 = j8, x9 = j9, x10 = j10, x11 = j11, x12 = j12, x13 = j13, x14 = j14, x15 = j15, u, i = 0; i < 20; i += 2)
            u = x0 + x12 | 0, x4 ^= u << 7 | u >>> 25, u = x4 + x0 | 0, x8 ^= u << 9 | u >>> 23, u = x8 + x4 | 0, x12 ^= u << 13 | u >>> 19, u = x12 + x8 | 0, x0 ^= u << 18 | u >>> 14, u = x5 + x1 | 0, x9 ^= u << 7 | u >>> 25, u = x9 + x5 | 0, x13 ^= u << 9 | u >>> 23, u = x13 + x9 | 0, x1 ^= u << 13 | u >>> 19, u = x1 + x13 | 0, x5 ^= u << 18 | u >>> 14, u = x10 + x6 | 0, x14 ^= u << 7 | u >>> 25, u = x14 + x10 | 0, x2 ^= u << 9 | u >>> 23, u = x2 + x14 | 0, x6 ^= u << 13 | u >>> 19, u = x6 + x2 | 0, x10 ^= u << 18 | u >>> 14, u = x15 + x11 | 0, x3 ^= u << 7 | u >>> 25, u = x3 + x15 | 0, x7 ^= u << 9 | u >>> 23, u = x7 + x3 | 0, x11 ^= u << 13 | u >>> 19, u = x11 + x7 | 0, x15 ^= u << 18 | u >>> 14, u = x0 + x3 | 0, x1 ^= u << 7 | u >>> 25, u = x1 + x0 | 0, x2 ^= u << 9 | u >>> 23, u = x2 + x1 | 0, x3 ^= u << 13 | u >>> 19, u = x3 + x2 | 0, x0 ^= u << 18 | u >>> 14, u = x5 + x4 | 0, x6 ^= u << 7 | u >>> 25, u = x6 + x5 | 0, x7 ^= u << 9 | u >>> 23, u = x7 + x6 | 0, x4 ^= u << 13 | u >>> 19, u = x4 + x7 | 0, x5 ^= u << 18 | u >>> 14, u = x10 + x9 | 0, x11 ^= u << 7 | u >>> 25, u = x11 + x10 | 0, x8 ^= u << 9 | u >>> 23, u = x8 + x11 | 0, x9 ^= u << 13 | u >>> 19, u = x9 + x8 | 0, x10 ^= u << 18 | u >>> 14, u = x15 + x14 | 0, x12 ^= u << 7 | u >>> 25, u = x12 + x15 | 0, x13 ^= u << 9 | u >>> 23, u = x13 + x12 | 0, x14 ^= u << 13 | u >>> 19, u = x14 + x13 | 0, x15 ^= u << 18 | u >>> 14;
          o[0] = x0 >>> 0 & 255, o[1] = x0 >>> 8 & 255, o[2] = x0 >>> 16 & 255, o[3] = x0 >>> 24 & 255, o[4] = x5 >>> 0 & 255, o[5] = x5 >>> 8 & 255, o[6] = x5 >>> 16 & 255, o[7] = x5 >>> 24 & 255, o[8] = x10 >>> 0 & 255, o[9] = x10 >>> 8 & 255, o[10] = x10 >>> 16 & 255, o[11] = x10 >>> 24 & 255, o[12] = x15 >>> 0 & 255, o[13] = x15 >>> 8 & 255, o[14] = x15 >>> 16 & 255, o[15] = x15 >>> 24 & 255, o[16] = x6 >>> 0 & 255, o[17] = x6 >>> 8 & 255, o[18] = x6 >>> 16 & 255, o[19] = x6 >>> 24 & 255, o[20] = x7 >>> 0 & 255, o[21] = x7 >>> 8 & 255, o[22] = x7 >>> 16 & 255, o[23] = x7 >>> 24 & 255, o[24] = x8 >>> 0 & 255, o[25] = x8 >>> 8 & 255, o[26] = x8 >>> 16 & 255, o[27] = x8 >>> 24 & 255, o[28] = x9 >>> 0 & 255, o[29] = x9 >>> 8 & 255, o[30] = x9 >>> 16 & 255, o[31] = x9 >>> 24 & 255;
        }
        function crypto_core_salsa20(out, inp, k, c) {
          core_salsa20(out, inp, k, c);
        }
        function crypto_core_hsalsa20(out, inp, k, c) {
          core_hsalsa20(out, inp, k, c);
        }
        var sigma = new Uint8Array([101, 120, 112, 97, 110, 100, 32, 51, 50, 45, 98, 121, 116, 101, 32, 107]);
        function crypto_stream_salsa20_xor(c, cpos, m, mpos, b, n, k) {
          var z = new Uint8Array(16), x = new Uint8Array(64), u, i;
          for (i = 0; i < 16; i++) z[i] = 0;
          for (i = 0; i < 8; i++) z[i] = n[i];
          for (; b >= 64; ) {
            for (crypto_core_salsa20(x, z, k, sigma), i = 0; i < 64; i++) c[cpos + i] = m[mpos + i] ^ x[i];
            for (u = 1, i = 8; i < 16; i++)
              u = u + (z[i] & 255) | 0, z[i] = u & 255, u >>>= 8;
            b -= 64, cpos += 64, mpos += 64;
          }
          if (b > 0)
            for (crypto_core_salsa20(x, z, k, sigma), i = 0; i < b; i++) c[cpos + i] = m[mpos + i] ^ x[i];
          return 0;
        }
        function crypto_stream_salsa20(c, cpos, b, n, k) {
          var z = new Uint8Array(16), x = new Uint8Array(64), u, i;
          for (i = 0; i < 16; i++) z[i] = 0;
          for (i = 0; i < 8; i++) z[i] = n[i];
          for (; b >= 64; ) {
            for (crypto_core_salsa20(x, z, k, sigma), i = 0; i < 64; i++) c[cpos + i] = x[i];
            for (u = 1, i = 8; i < 16; i++)
              u = u + (z[i] & 255) | 0, z[i] = u & 255, u >>>= 8;
            b -= 64, cpos += 64;
          }
          if (b > 0)
            for (crypto_core_salsa20(x, z, k, sigma), i = 0; i < b; i++) c[cpos + i] = x[i];
          return 0;
        }
        function crypto_stream(c, cpos, d, n, k) {
          var s = new Uint8Array(32);
          crypto_core_hsalsa20(s, n, k, sigma);
          for (var sn = new Uint8Array(8), i = 0; i < 8; i++) sn[i] = n[i + 16];
          return crypto_stream_salsa20(c, cpos, d, sn, s);
        }
        function crypto_stream_xor(c, cpos, m, mpos, d, n, k) {
          var s = new Uint8Array(32);
          crypto_core_hsalsa20(s, n, k, sigma);
          for (var sn = new Uint8Array(8), i = 0; i < 8; i++) sn[i] = n[i + 16];
          return crypto_stream_salsa20_xor(c, cpos, m, mpos, d, sn, s);
        }
        var poly1305 = function(key) {
          this.buffer = new Uint8Array(16), this.r = new Uint16Array(10), this.h = new Uint16Array(10), this.pad = new Uint16Array(8), this.leftover = 0, this.fin = 0;
          var t0, t1, t2, t3, t4, t5, t6, t7;
          t0 = key[0] & 255 | (key[1] & 255) << 8, this.r[0] = t0 & 8191, t1 = key[2] & 255 | (key[3] & 255) << 8, this.r[1] = (t0 >>> 13 | t1 << 3) & 8191, t2 = key[4] & 255 | (key[5] & 255) << 8, this.r[2] = (t1 >>> 10 | t2 << 6) & 7939, t3 = key[6] & 255 | (key[7] & 255) << 8, this.r[3] = (t2 >>> 7 | t3 << 9) & 8191, t4 = key[8] & 255 | (key[9] & 255) << 8, this.r[4] = (t3 >>> 4 | t4 << 12) & 255, this.r[5] = t4 >>> 1 & 8190, t5 = key[10] & 255 | (key[11] & 255) << 8, this.r[6] = (t4 >>> 14 | t5 << 2) & 8191, t6 = key[12] & 255 | (key[13] & 255) << 8, this.r[7] = (t5 >>> 11 | t6 << 5) & 8065, t7 = key[14] & 255 | (key[15] & 255) << 8, this.r[8] = (t6 >>> 8 | t7 << 8) & 8191, this.r[9] = t7 >>> 5 & 127, this.pad[0] = key[16] & 255 | (key[17] & 255) << 8, this.pad[1] = key[18] & 255 | (key[19] & 255) << 8, this.pad[2] = key[20] & 255 | (key[21] & 255) << 8, this.pad[3] = key[22] & 255 | (key[23] & 255) << 8, this.pad[4] = key[24] & 255 | (key[25] & 255) << 8, this.pad[5] = key[26] & 255 | (key[27] & 255) << 8, this.pad[6] = key[28] & 255 | (key[29] & 255) << 8, this.pad[7] = key[30] & 255 | (key[31] & 255) << 8;
        };
        poly1305.prototype.blocks = function(m, mpos, bytes) {
          for (var hibit = this.fin ? 0 : 2048, t0, t1, t2, t3, t4, t5, t6, t7, c, d0, d1, d2, d3, d4, d5, d6, d7, d8, d9, h0 = this.h[0], h1 = this.h[1], h2 = this.h[2], h3 = this.h[3], h4 = this.h[4], h5 = this.h[5], h6 = this.h[6], h7 = this.h[7], h8 = this.h[8], h9 = this.h[9], r0 = this.r[0], r1 = this.r[1], r2 = this.r[2], r3 = this.r[3], r4 = this.r[4], r5 = this.r[5], r6 = this.r[6], r7 = this.r[7], r8 = this.r[8], r9 = this.r[9]; bytes >= 16; )
            t0 = m[mpos + 0] & 255 | (m[mpos + 1] & 255) << 8, h0 += t0 & 8191, t1 = m[mpos + 2] & 255 | (m[mpos + 3] & 255) << 8, h1 += (t0 >>> 13 | t1 << 3) & 8191, t2 = m[mpos + 4] & 255 | (m[mpos + 5] & 255) << 8, h2 += (t1 >>> 10 | t2 << 6) & 8191, t3 = m[mpos + 6] & 255 | (m[mpos + 7] & 255) << 8, h3 += (t2 >>> 7 | t3 << 9) & 8191, t4 = m[mpos + 8] & 255 | (m[mpos + 9] & 255) << 8, h4 += (t3 >>> 4 | t4 << 12) & 8191, h5 += t4 >>> 1 & 8191, t5 = m[mpos + 10] & 255 | (m[mpos + 11] & 255) << 8, h6 += (t4 >>> 14 | t5 << 2) & 8191, t6 = m[mpos + 12] & 255 | (m[mpos + 13] & 255) << 8, h7 += (t5 >>> 11 | t6 << 5) & 8191, t7 = m[mpos + 14] & 255 | (m[mpos + 15] & 255) << 8, h8 += (t6 >>> 8 | t7 << 8) & 8191, h9 += t7 >>> 5 | hibit, c = 0, d0 = c, d0 += h0 * r0, d0 += h1 * (5 * r9), d0 += h2 * (5 * r8), d0 += h3 * (5 * r7), d0 += h4 * (5 * r6), c = d0 >>> 13, d0 &= 8191, d0 += h5 * (5 * r5), d0 += h6 * (5 * r4), d0 += h7 * (5 * r3), d0 += h8 * (5 * r2), d0 += h9 * (5 * r1), c += d0 >>> 13, d0 &= 8191, d1 = c, d1 += h0 * r1, d1 += h1 * r0, d1 += h2 * (5 * r9), d1 += h3 * (5 * r8), d1 += h4 * (5 * r7), c = d1 >>> 13, d1 &= 8191, d1 += h5 * (5 * r6), d1 += h6 * (5 * r5), d1 += h7 * (5 * r4), d1 += h8 * (5 * r3), d1 += h9 * (5 * r2), c += d1 >>> 13, d1 &= 8191, d2 = c, d2 += h0 * r2, d2 += h1 * r1, d2 += h2 * r0, d2 += h3 * (5 * r9), d2 += h4 * (5 * r8), c = d2 >>> 13, d2 &= 8191, d2 += h5 * (5 * r7), d2 += h6 * (5 * r6), d2 += h7 * (5 * r5), d2 += h8 * (5 * r4), d2 += h9 * (5 * r3), c += d2 >>> 13, d2 &= 8191, d3 = c, d3 += h0 * r3, d3 += h1 * r2, d3 += h2 * r1, d3 += h3 * r0, d3 += h4 * (5 * r9), c = d3 >>> 13, d3 &= 8191, d3 += h5 * (5 * r8), d3 += h6 * (5 * r7), d3 += h7 * (5 * r6), d3 += h8 * (5 * r5), d3 += h9 * (5 * r4), c += d3 >>> 13, d3 &= 8191, d4 = c, d4 += h0 * r4, d4 += h1 * r3, d4 += h2 * r2, d4 += h3 * r1, d4 += h4 * r0, c = d4 >>> 13, d4 &= 8191, d4 += h5 * (5 * r9), d4 += h6 * (5 * r8), d4 += h7 * (5 * r7), d4 += h8 * (5 * r6), d4 += h9 * (5 * r5), c += d4 >>> 13, d4 &= 8191, d5 = c, d5 += h0 * r5, d5 += h1 * r4, d5 += h2 * r3, d5 += h3 * r2, d5 += h4 * r1, c = d5 >>> 13, d5 &= 8191, d5 += h5 * r0, d5 += h6 * (5 * r9), d5 += h7 * (5 * r8), d5 += h8 * (5 * r7), d5 += h9 * (5 * r6), c += d5 >>> 13, d5 &= 8191, d6 = c, d6 += h0 * r6, d6 += h1 * r5, d6 += h2 * r4, d6 += h3 * r3, d6 += h4 * r2, c = d6 >>> 13, d6 &= 8191, d6 += h5 * r1, d6 += h6 * r0, d6 += h7 * (5 * r9), d6 += h8 * (5 * r8), d6 += h9 * (5 * r7), c += d6 >>> 13, d6 &= 8191, d7 = c, d7 += h0 * r7, d7 += h1 * r6, d7 += h2 * r5, d7 += h3 * r4, d7 += h4 * r3, c = d7 >>> 13, d7 &= 8191, d7 += h5 * r2, d7 += h6 * r1, d7 += h7 * r0, d7 += h8 * (5 * r9), d7 += h9 * (5 * r8), c += d7 >>> 13, d7 &= 8191, d8 = c, d8 += h0 * r8, d8 += h1 * r7, d8 += h2 * r6, d8 += h3 * r5, d8 += h4 * r4, c = d8 >>> 13, d8 &= 8191, d8 += h5 * r3, d8 += h6 * r2, d8 += h7 * r1, d8 += h8 * r0, d8 += h9 * (5 * r9), c += d8 >>> 13, d8 &= 8191, d9 = c, d9 += h0 * r9, d9 += h1 * r8, d9 += h2 * r7, d9 += h3 * r6, d9 += h4 * r5, c = d9 >>> 13, d9 &= 8191, d9 += h5 * r4, d9 += h6 * r3, d9 += h7 * r2, d9 += h8 * r1, d9 += h9 * r0, c += d9 >>> 13, d9 &= 8191, c = (c << 2) + c | 0, c = c + d0 | 0, d0 = c & 8191, c = c >>> 13, d1 += c, h0 = d0, h1 = d1, h2 = d2, h3 = d3, h4 = d4, h5 = d5, h6 = d6, h7 = d7, h8 = d8, h9 = d9, mpos += 16, bytes -= 16;
          this.h[0] = h0, this.h[1] = h1, this.h[2] = h2, this.h[3] = h3, this.h[4] = h4, this.h[5] = h5, this.h[6] = h6, this.h[7] = h7, this.h[8] = h8, this.h[9] = h9;
        }, poly1305.prototype.finish = function(mac, macpos) {
          var g = new Uint16Array(10), c, mask, f, i;
          if (this.leftover) {
            for (i = this.leftover, this.buffer[i++] = 1; i < 16; i++) this.buffer[i] = 0;
            this.fin = 1, this.blocks(this.buffer, 0, 16);
          }
          for (c = this.h[1] >>> 13, this.h[1] &= 8191, i = 2; i < 10; i++)
            this.h[i] += c, c = this.h[i] >>> 13, this.h[i] &= 8191;
          for (this.h[0] += c * 5, c = this.h[0] >>> 13, this.h[0] &= 8191, this.h[1] += c, c = this.h[1] >>> 13, this.h[1] &= 8191, this.h[2] += c, g[0] = this.h[0] + 5, c = g[0] >>> 13, g[0] &= 8191, i = 1; i < 10; i++)
            g[i] = this.h[i] + c, c = g[i] >>> 13, g[i] &= 8191;
          for (g[9] -= 8192, mask = (c ^ 1) - 1, i = 0; i < 10; i++) g[i] &= mask;
          for (mask = ~mask, i = 0; i < 10; i++) this.h[i] = this.h[i] & mask | g[i];
          for (this.h[0] = (this.h[0] | this.h[1] << 13) & 65535, this.h[1] = (this.h[1] >>> 3 | this.h[2] << 10) & 65535, this.h[2] = (this.h[2] >>> 6 | this.h[3] << 7) & 65535, this.h[3] = (this.h[3] >>> 9 | this.h[4] << 4) & 65535, this.h[4] = (this.h[4] >>> 12 | this.h[5] << 1 | this.h[6] << 14) & 65535, this.h[5] = (this.h[6] >>> 2 | this.h[7] << 11) & 65535, this.h[6] = (this.h[7] >>> 5 | this.h[8] << 8) & 65535, this.h[7] = (this.h[8] >>> 8 | this.h[9] << 5) & 65535, f = this.h[0] + this.pad[0], this.h[0] = f & 65535, i = 1; i < 8; i++)
            f = (this.h[i] + this.pad[i] | 0) + (f >>> 16) | 0, this.h[i] = f & 65535;
          mac[macpos + 0] = this.h[0] >>> 0 & 255, mac[macpos + 1] = this.h[0] >>> 8 & 255, mac[macpos + 2] = this.h[1] >>> 0 & 255, mac[macpos + 3] = this.h[1] >>> 8 & 255, mac[macpos + 4] = this.h[2] >>> 0 & 255, mac[macpos + 5] = this.h[2] >>> 8 & 255, mac[macpos + 6] = this.h[3] >>> 0 & 255, mac[macpos + 7] = this.h[3] >>> 8 & 255, mac[macpos + 8] = this.h[4] >>> 0 & 255, mac[macpos + 9] = this.h[4] >>> 8 & 255, mac[macpos + 10] = this.h[5] >>> 0 & 255, mac[macpos + 11] = this.h[5] >>> 8 & 255, mac[macpos + 12] = this.h[6] >>> 0 & 255, mac[macpos + 13] = this.h[6] >>> 8 & 255, mac[macpos + 14] = this.h[7] >>> 0 & 255, mac[macpos + 15] = this.h[7] >>> 8 & 255;
        }, poly1305.prototype.update = function(m, mpos, bytes) {
          var i, want;
          if (this.leftover) {
            for (want = 16 - this.leftover, want > bytes && (want = bytes), i = 0; i < want; i++)
              this.buffer[this.leftover + i] = m[mpos + i];
            if (bytes -= want, mpos += want, this.leftover += want, this.leftover < 16)
              return;
            this.blocks(this.buffer, 0, 16), this.leftover = 0;
          }
          if (bytes >= 16 && (want = bytes - bytes % 16, this.blocks(m, mpos, want), mpos += want, bytes -= want), bytes) {
            for (i = 0; i < bytes; i++)
              this.buffer[this.leftover + i] = m[mpos + i];
            this.leftover += bytes;
          }
        };
        function crypto_onetimeauth(out, outpos, m, mpos, n, k) {
          var s = new poly1305(k);
          return s.update(m, mpos, n), s.finish(out, outpos), 0;
        }
        function crypto_onetimeauth_verify(h, hpos, m, mpos, n, k) {
          var x = new Uint8Array(16);
          return crypto_onetimeauth(x, 0, m, mpos, n, k), crypto_verify_16(h, hpos, x, 0);
        }
        function crypto_secretbox(c, m, d, n, k) {
          var i;
          if (d < 32) return -1;
          for (crypto_stream_xor(c, 0, m, 0, d, n, k), crypto_onetimeauth(c, 16, c, 32, d - 32, c), i = 0; i < 16; i++) c[i] = 0;
          return 0;
        }
        function crypto_secretbox_open(m, c, d, n, k) {
          var i, x = new Uint8Array(32);
          if (d < 32 || (crypto_stream(x, 0, 32, n, k), crypto_onetimeauth_verify(c, 16, c, 32, d - 32, x) !== 0)) return -1;
          for (crypto_stream_xor(m, 0, c, 0, d, n, k), i = 0; i < 32; i++) m[i] = 0;
          return 0;
        }
        function set25519(r, a) {
          var i;
          for (i = 0; i < 16; i++) r[i] = a[i] | 0;
        }
        function car25519(o) {
          var i, v, c = 1;
          for (i = 0; i < 16; i++)
            v = o[i] + c + 65535, c = Math.floor(v / 65536), o[i] = v - c * 65536;
          o[0] += c - 1 + 37 * (c - 1);
        }
        function sel25519(p, q, b) {
          for (var t, c = ~(b - 1), i = 0; i < 16; i++)
            t = c & (p[i] ^ q[i]), p[i] ^= t, q[i] ^= t;
        }
        function pack25519(o, n) {
          var i, j, b, m = gf(), t = gf();
          for (i = 0; i < 16; i++) t[i] = n[i];
          for (car25519(t), car25519(t), car25519(t), j = 0; j < 2; j++) {
            for (m[0] = t[0] - 65517, i = 1; i < 15; i++)
              m[i] = t[i] - 65535 - (m[i - 1] >> 16 & 1), m[i - 1] &= 65535;
            m[15] = t[15] - 32767 - (m[14] >> 16 & 1), b = m[15] >> 16 & 1, m[14] &= 65535, sel25519(t, m, 1 - b);
          }
          for (i = 0; i < 16; i++)
            o[2 * i] = t[i] & 255, o[2 * i + 1] = t[i] >> 8;
        }
        function neq25519(a, b) {
          var c = new Uint8Array(32), d = new Uint8Array(32);
          return pack25519(c, a), pack25519(d, b), crypto_verify_32(c, 0, d, 0);
        }
        function par25519(a) {
          var d = new Uint8Array(32);
          return pack25519(d, a), d[0] & 1;
        }
        function unpack25519(o, n) {
          var i;
          for (i = 0; i < 16; i++) o[i] = n[2 * i] + (n[2 * i + 1] << 8);
          o[15] &= 32767;
        }
        function A(o, a, b) {
          for (var i = 0; i < 16; i++) o[i] = a[i] + b[i];
        }
        function Z(o, a, b) {
          for (var i = 0; i < 16; i++) o[i] = a[i] - b[i];
        }
        function M(o, a, b) {
          var v, c, t0 = 0, t1 = 0, t2 = 0, t3 = 0, t4 = 0, t5 = 0, t6 = 0, t7 = 0, t8 = 0, t9 = 0, t10 = 0, t11 = 0, t12 = 0, t13 = 0, t14 = 0, t15 = 0, t16 = 0, t17 = 0, t18 = 0, t19 = 0, t20 = 0, t21 = 0, t22 = 0, t23 = 0, t24 = 0, t25 = 0, t26 = 0, t27 = 0, t28 = 0, t29 = 0, t30 = 0, b0 = b[0], b1 = b[1], b2 = b[2], b3 = b[3], b4 = b[4], b5 = b[5], b6 = b[6], b7 = b[7], b8 = b[8], b9 = b[9], b10 = b[10], b11 = b[11], b12 = b[12], b13 = b[13], b14 = b[14], b15 = b[15];
          v = a[0], t0 += v * b0, t1 += v * b1, t2 += v * b2, t3 += v * b3, t4 += v * b4, t5 += v * b5, t6 += v * b6, t7 += v * b7, t8 += v * b8, t9 += v * b9, t10 += v * b10, t11 += v * b11, t12 += v * b12, t13 += v * b13, t14 += v * b14, t15 += v * b15, v = a[1], t1 += v * b0, t2 += v * b1, t3 += v * b2, t4 += v * b3, t5 += v * b4, t6 += v * b5, t7 += v * b6, t8 += v * b7, t9 += v * b8, t10 += v * b9, t11 += v * b10, t12 += v * b11, t13 += v * b12, t14 += v * b13, t15 += v * b14, t16 += v * b15, v = a[2], t2 += v * b0, t3 += v * b1, t4 += v * b2, t5 += v * b3, t6 += v * b4, t7 += v * b5, t8 += v * b6, t9 += v * b7, t10 += v * b8, t11 += v * b9, t12 += v * b10, t13 += v * b11, t14 += v * b12, t15 += v * b13, t16 += v * b14, t17 += v * b15, v = a[3], t3 += v * b0, t4 += v * b1, t5 += v * b2, t6 += v * b3, t7 += v * b4, t8 += v * b5, t9 += v * b6, t10 += v * b7, t11 += v * b8, t12 += v * b9, t13 += v * b10, t14 += v * b11, t15 += v * b12, t16 += v * b13, t17 += v * b14, t18 += v * b15, v = a[4], t4 += v * b0, t5 += v * b1, t6 += v * b2, t7 += v * b3, t8 += v * b4, t9 += v * b5, t10 += v * b6, t11 += v * b7, t12 += v * b8, t13 += v * b9, t14 += v * b10, t15 += v * b11, t16 += v * b12, t17 += v * b13, t18 += v * b14, t19 += v * b15, v = a[5], t5 += v * b0, t6 += v * b1, t7 += v * b2, t8 += v * b3, t9 += v * b4, t10 += v * b5, t11 += v * b6, t12 += v * b7, t13 += v * b8, t14 += v * b9, t15 += v * b10, t16 += v * b11, t17 += v * b12, t18 += v * b13, t19 += v * b14, t20 += v * b15, v = a[6], t6 += v * b0, t7 += v * b1, t8 += v * b2, t9 += v * b3, t10 += v * b4, t11 += v * b5, t12 += v * b6, t13 += v * b7, t14 += v * b8, t15 += v * b9, t16 += v * b10, t17 += v * b11, t18 += v * b12, t19 += v * b13, t20 += v * b14, t21 += v * b15, v = a[7], t7 += v * b0, t8 += v * b1, t9 += v * b2, t10 += v * b3, t11 += v * b4, t12 += v * b5, t13 += v * b6, t14 += v * b7, t15 += v * b8, t16 += v * b9, t17 += v * b10, t18 += v * b11, t19 += v * b12, t20 += v * b13, t21 += v * b14, t22 += v * b15, v = a[8], t8 += v * b0, t9 += v * b1, t10 += v * b2, t11 += v * b3, t12 += v * b4, t13 += v * b5, t14 += v * b6, t15 += v * b7, t16 += v * b8, t17 += v * b9, t18 += v * b10, t19 += v * b11, t20 += v * b12, t21 += v * b13, t22 += v * b14, t23 += v * b15, v = a[9], t9 += v * b0, t10 += v * b1, t11 += v * b2, t12 += v * b3, t13 += v * b4, t14 += v * b5, t15 += v * b6, t16 += v * b7, t17 += v * b8, t18 += v * b9, t19 += v * b10, t20 += v * b11, t21 += v * b12, t22 += v * b13, t23 += v * b14, t24 += v * b15, v = a[10], t10 += v * b0, t11 += v * b1, t12 += v * b2, t13 += v * b3, t14 += v * b4, t15 += v * b5, t16 += v * b6, t17 += v * b7, t18 += v * b8, t19 += v * b9, t20 += v * b10, t21 += v * b11, t22 += v * b12, t23 += v * b13, t24 += v * b14, t25 += v * b15, v = a[11], t11 += v * b0, t12 += v * b1, t13 += v * b2, t14 += v * b3, t15 += v * b4, t16 += v * b5, t17 += v * b6, t18 += v * b7, t19 += v * b8, t20 += v * b9, t21 += v * b10, t22 += v * b11, t23 += v * b12, t24 += v * b13, t25 += v * b14, t26 += v * b15, v = a[12], t12 += v * b0, t13 += v * b1, t14 += v * b2, t15 += v * b3, t16 += v * b4, t17 += v * b5, t18 += v * b6, t19 += v * b7, t20 += v * b8, t21 += v * b9, t22 += v * b10, t23 += v * b11, t24 += v * b12, t25 += v * b13, t26 += v * b14, t27 += v * b15, v = a[13], t13 += v * b0, t14 += v * b1, t15 += v * b2, t16 += v * b3, t17 += v * b4, t18 += v * b5, t19 += v * b6, t20 += v * b7, t21 += v * b8, t22 += v * b9, t23 += v * b10, t24 += v * b11, t25 += v * b12, t26 += v * b13, t27 += v * b14, t28 += v * b15, v = a[14], t14 += v * b0, t15 += v * b1, t16 += v * b2, t17 += v * b3, t18 += v * b4, t19 += v * b5, t20 += v * b6, t21 += v * b7, t22 += v * b8, t23 += v * b9, t24 += v * b10, t25 += v * b11, t26 += v * b12, t27 += v * b13, t28 += v * b14, t29 += v * b15, v = a[15], t15 += v * b0, t16 += v * b1, t17 += v * b2, t18 += v * b3, t19 += v * b4, t20 += v * b5, t21 += v * b6, t22 += v * b7, t23 += v * b8, t24 += v * b9, t25 += v * b10, t26 += v * b11, t27 += v * b12, t28 += v * b13, t29 += v * b14, t30 += v * b15, t0 += 38 * t16, t1 += 38 * t17, t2 += 38 * t18, t3 += 38 * t19, t4 += 38 * t20, t5 += 38 * t21, t6 += 38 * t22, t7 += 38 * t23, t8 += 38 * t24, t9 += 38 * t25, t10 += 38 * t26, t11 += 38 * t27, t12 += 38 * t28, t13 += 38 * t29, t14 += 38 * t30, c = 1, v = t0 + c + 65535, c = Math.floor(v / 65536), t0 = v - c * 65536, v = t1 + c + 65535, c = Math.floor(v / 65536), t1 = v - c * 65536, v = t2 + c + 65535, c = Math.floor(v / 65536), t2 = v - c * 65536, v = t3 + c + 65535, c = Math.floor(v / 65536), t3 = v - c * 65536, v = t4 + c + 65535, c = Math.floor(v / 65536), t4 = v - c * 65536, v = t5 + c + 65535, c = Math.floor(v / 65536), t5 = v - c * 65536, v = t6 + c + 65535, c = Math.floor(v / 65536), t6 = v - c * 65536, v = t7 + c + 65535, c = Math.floor(v / 65536), t7 = v - c * 65536, v = t8 + c + 65535, c = Math.floor(v / 65536), t8 = v - c * 65536, v = t9 + c + 65535, c = Math.floor(v / 65536), t9 = v - c * 65536, v = t10 + c + 65535, c = Math.floor(v / 65536), t10 = v - c * 65536, v = t11 + c + 65535, c = Math.floor(v / 65536), t11 = v - c * 65536, v = t12 + c + 65535, c = Math.floor(v / 65536), t12 = v - c * 65536, v = t13 + c + 65535, c = Math.floor(v / 65536), t13 = v - c * 65536, v = t14 + c + 65535, c = Math.floor(v / 65536), t14 = v - c * 65536, v = t15 + c + 65535, c = Math.floor(v / 65536), t15 = v - c * 65536, t0 += c - 1 + 37 * (c - 1), c = 1, v = t0 + c + 65535, c = Math.floor(v / 65536), t0 = v - c * 65536, v = t1 + c + 65535, c = Math.floor(v / 65536), t1 = v - c * 65536, v = t2 + c + 65535, c = Math.floor(v / 65536), t2 = v - c * 65536, v = t3 + c + 65535, c = Math.floor(v / 65536), t3 = v - c * 65536, v = t4 + c + 65535, c = Math.floor(v / 65536), t4 = v - c * 65536, v = t5 + c + 65535, c = Math.floor(v / 65536), t5 = v - c * 65536, v = t6 + c + 65535, c = Math.floor(v / 65536), t6 = v - c * 65536, v = t7 + c + 65535, c = Math.floor(v / 65536), t7 = v - c * 65536, v = t8 + c + 65535, c = Math.floor(v / 65536), t8 = v - c * 65536, v = t9 + c + 65535, c = Math.floor(v / 65536), t9 = v - c * 65536, v = t10 + c + 65535, c = Math.floor(v / 65536), t10 = v - c * 65536, v = t11 + c + 65535, c = Math.floor(v / 65536), t11 = v - c * 65536, v = t12 + c + 65535, c = Math.floor(v / 65536), t12 = v - c * 65536, v = t13 + c + 65535, c = Math.floor(v / 65536), t13 = v - c * 65536, v = t14 + c + 65535, c = Math.floor(v / 65536), t14 = v - c * 65536, v = t15 + c + 65535, c = Math.floor(v / 65536), t15 = v - c * 65536, t0 += c - 1 + 37 * (c - 1), o[0] = t0, o[1] = t1, o[2] = t2, o[3] = t3, o[4] = t4, o[5] = t5, o[6] = t6, o[7] = t7, o[8] = t8, o[9] = t9, o[10] = t10, o[11] = t11, o[12] = t12, o[13] = t13, o[14] = t14, o[15] = t15;
        }
        function S(o, a) {
          M(o, a, a);
        }
        function inv25519(o, i) {
          var c = gf(), a;
          for (a = 0; a < 16; a++) c[a] = i[a];
          for (a = 253; a >= 0; a--)
            S(c, c), a !== 2 && a !== 4 && M(c, c, i);
          for (a = 0; a < 16; a++) o[a] = c[a];
        }
        function pow2523(o, i) {
          var c = gf(), a;
          for (a = 0; a < 16; a++) c[a] = i[a];
          for (a = 250; a >= 0; a--)
            S(c, c), a !== 1 && M(c, c, i);
          for (a = 0; a < 16; a++) o[a] = c[a];
        }
        function crypto_scalarmult(q, n, p) {
          var z = new Uint8Array(32), x = new Float64Array(80), r, i, a = gf(), b = gf(), c = gf(), d = gf(), e = gf(), f = gf();
          for (i = 0; i < 31; i++) z[i] = n[i];
          for (z[31] = n[31] & 127 | 64, z[0] &= 248, unpack25519(x, p), i = 0; i < 16; i++)
            b[i] = x[i], d[i] = a[i] = c[i] = 0;
          for (a[0] = d[0] = 1, i = 254; i >= 0; --i)
            r = z[i >>> 3] >>> (i & 7) & 1, sel25519(a, b, r), sel25519(c, d, r), A(e, a, c), Z(a, a, c), A(c, b, d), Z(b, b, d), S(d, e), S(f, a), M(a, c, a), M(c, b, e), A(e, a, c), Z(a, a, c), S(b, a), Z(c, d, f), M(a, c, _121665), A(a, a, d), M(c, c, a), M(a, d, f), M(d, b, x), S(b, e), sel25519(a, b, r), sel25519(c, d, r);
          for (i = 0; i < 16; i++)
            x[i + 16] = a[i], x[i + 32] = c[i], x[i + 48] = b[i], x[i + 64] = d[i];
          var x32 = x.subarray(32), x16 = x.subarray(16);
          return inv25519(x32, x32), M(x16, x16, x32), pack25519(q, x16), 0;
        }
        function crypto_scalarmult_base(q, n) {
          return crypto_scalarmult(q, n, _9);
        }
        function crypto_box_keypair(y, x) {
          return randombytes(x, 32), crypto_scalarmult_base(y, x);
        }
        function crypto_box_beforenm(k, y, x) {
          var s = new Uint8Array(32);
          return crypto_scalarmult(s, x, y), crypto_core_hsalsa20(k, _0, s, sigma);
        }
        var crypto_box_afternm = crypto_secretbox, crypto_box_open_afternm = crypto_secretbox_open;
        function crypto_box(c, m, d, n, y, x) {
          var k = new Uint8Array(32);
          return crypto_box_beforenm(k, y, x), crypto_box_afternm(c, m, d, n, k);
        }
        function crypto_box_open(m, c, d, n, y, x) {
          var k = new Uint8Array(32);
          return crypto_box_beforenm(k, y, x), crypto_box_open_afternm(m, c, d, n, k);
        }
        var K = [
          1116352408,
          3609767458,
          1899447441,
          602891725,
          3049323471,
          3964484399,
          3921009573,
          2173295548,
          961987163,
          4081628472,
          1508970993,
          3053834265,
          2453635748,
          2937671579,
          2870763221,
          3664609560,
          3624381080,
          2734883394,
          310598401,
          1164996542,
          607225278,
          1323610764,
          1426881987,
          3590304994,
          1925078388,
          4068182383,
          2162078206,
          991336113,
          2614888103,
          633803317,
          3248222580,
          3479774868,
          3835390401,
          2666613458,
          4022224774,
          944711139,
          264347078,
          2341262773,
          604807628,
          2007800933,
          770255983,
          1495990901,
          1249150122,
          1856431235,
          1555081692,
          3175218132,
          1996064986,
          2198950837,
          2554220882,
          3999719339,
          2821834349,
          766784016,
          2952996808,
          2566594879,
          3210313671,
          3203337956,
          3336571891,
          1034457026,
          3584528711,
          2466948901,
          113926993,
          3758326383,
          338241895,
          168717936,
          666307205,
          1188179964,
          773529912,
          1546045734,
          1294757372,
          1522805485,
          1396182291,
          2643833823,
          1695183700,
          2343527390,
          1986661051,
          1014477480,
          2177026350,
          1206759142,
          2456956037,
          344077627,
          2730485921,
          1290863460,
          2820302411,
          3158454273,
          3259730800,
          3505952657,
          3345764771,
          106217008,
          3516065817,
          3606008344,
          3600352804,
          1432725776,
          4094571909,
          1467031594,
          275423344,
          851169720,
          430227734,
          3100823752,
          506948616,
          1363258195,
          659060556,
          3750685593,
          883997877,
          3785050280,
          958139571,
          3318307427,
          1322822218,
          3812723403,
          1537002063,
          2003034995,
          1747873779,
          3602036899,
          1955562222,
          1575990012,
          2024104815,
          1125592928,
          2227730452,
          2716904306,
          2361852424,
          442776044,
          2428436474,
          593698344,
          2756734187,
          3733110249,
          3204031479,
          2999351573,
          3329325298,
          3815920427,
          3391569614,
          3928383900,
          3515267271,
          566280711,
          3940187606,
          3454069534,
          4118630271,
          4000239992,
          116418474,
          1914138554,
          174292421,
          2731055270,
          289380356,
          3203993006,
          460393269,
          320620315,
          685471733,
          587496836,
          852142971,
          1086792851,
          1017036298,
          365543100,
          1126000580,
          2618297676,
          1288033470,
          3409855158,
          1501505948,
          4234509866,
          1607167915,
          987167468,
          1816402316,
          1246189591
        ];
        function crypto_hashblocks_hl(hh, hl, m, n) {
          for (var wh = new Int32Array(16), wl = new Int32Array(16), bh0, bh1, bh2, bh3, bh4, bh5, bh6, bh7, bl0, bl1, bl2, bl3, bl4, bl5, bl6, bl7, th, tl, i, j, h, l, a, b, c, d, ah0 = hh[0], ah1 = hh[1], ah2 = hh[2], ah3 = hh[3], ah4 = hh[4], ah5 = hh[5], ah6 = hh[6], ah7 = hh[7], al0 = hl[0], al1 = hl[1], al2 = hl[2], al3 = hl[3], al4 = hl[4], al5 = hl[5], al6 = hl[6], al7 = hl[7], pos = 0; n >= 128; ) {
            for (i = 0; i < 16; i++)
              j = 8 * i + pos, wh[i] = m[j + 0] << 24 | m[j + 1] << 16 | m[j + 2] << 8 | m[j + 3], wl[i] = m[j + 4] << 24 | m[j + 5] << 16 | m[j + 6] << 8 | m[j + 7];
            for (i = 0; i < 80; i++)
              if (bh0 = ah0, bh1 = ah1, bh2 = ah2, bh3 = ah3, bh4 = ah4, bh5 = ah5, bh6 = ah6, bh7 = ah7, bl0 = al0, bl1 = al1, bl2 = al2, bl3 = al3, bl4 = al4, bl5 = al5, bl6 = al6, bl7 = al7, h = ah7, l = al7, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = (ah4 >>> 14 | al4 << 18) ^ (ah4 >>> 18 | al4 << 14) ^ (al4 >>> 9 | ah4 << 23), l = (al4 >>> 14 | ah4 << 18) ^ (al4 >>> 18 | ah4 << 14) ^ (ah4 >>> 9 | al4 << 23), a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, h = ah4 & ah5 ^ ~ah4 & ah6, l = al4 & al5 ^ ~al4 & al6, a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, h = K[i * 2], l = K[i * 2 + 1], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, h = wh[i % 16], l = wl[i % 16], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, th = c & 65535 | d << 16, tl = a & 65535 | b << 16, h = th, l = tl, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = (ah0 >>> 28 | al0 << 4) ^ (al0 >>> 2 | ah0 << 30) ^ (al0 >>> 7 | ah0 << 25), l = (al0 >>> 28 | ah0 << 4) ^ (ah0 >>> 2 | al0 << 30) ^ (ah0 >>> 7 | al0 << 25), a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, h = ah0 & ah1 ^ ah0 & ah2 ^ ah1 & ah2, l = al0 & al1 ^ al0 & al2 ^ al1 & al2, a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, bh7 = c & 65535 | d << 16, bl7 = a & 65535 | b << 16, h = bh3, l = bl3, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = th, l = tl, a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, bh3 = c & 65535 | d << 16, bl3 = a & 65535 | b << 16, ah1 = bh0, ah2 = bh1, ah3 = bh2, ah4 = bh3, ah5 = bh4, ah6 = bh5, ah7 = bh6, ah0 = bh7, al1 = bl0, al2 = bl1, al3 = bl2, al4 = bl3, al5 = bl4, al6 = bl5, al7 = bl6, al0 = bl7, i % 16 === 15)
                for (j = 0; j < 16; j++)
                  h = wh[j], l = wl[j], a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = wh[(j + 9) % 16], l = wl[(j + 9) % 16], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, th = wh[(j + 1) % 16], tl = wl[(j + 1) % 16], h = (th >>> 1 | tl << 31) ^ (th >>> 8 | tl << 24) ^ th >>> 7, l = (tl >>> 1 | th << 31) ^ (tl >>> 8 | th << 24) ^ (tl >>> 7 | th << 25), a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, th = wh[(j + 14) % 16], tl = wl[(j + 14) % 16], h = (th >>> 19 | tl << 13) ^ (tl >>> 29 | th << 3) ^ th >>> 6, l = (tl >>> 19 | th << 13) ^ (th >>> 29 | tl << 3) ^ (tl >>> 6 | th << 26), a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, wh[j] = c & 65535 | d << 16, wl[j] = a & 65535 | b << 16;
            h = ah0, l = al0, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = hh[0], l = hl[0], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, hh[0] = ah0 = c & 65535 | d << 16, hl[0] = al0 = a & 65535 | b << 16, h = ah1, l = al1, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = hh[1], l = hl[1], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, hh[1] = ah1 = c & 65535 | d << 16, hl[1] = al1 = a & 65535 | b << 16, h = ah2, l = al2, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = hh[2], l = hl[2], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, hh[2] = ah2 = c & 65535 | d << 16, hl[2] = al2 = a & 65535 | b << 16, h = ah3, l = al3, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = hh[3], l = hl[3], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, hh[3] = ah3 = c & 65535 | d << 16, hl[3] = al3 = a & 65535 | b << 16, h = ah4, l = al4, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = hh[4], l = hl[4], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, hh[4] = ah4 = c & 65535 | d << 16, hl[4] = al4 = a & 65535 | b << 16, h = ah5, l = al5, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = hh[5], l = hl[5], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, hh[5] = ah5 = c & 65535 | d << 16, hl[5] = al5 = a & 65535 | b << 16, h = ah6, l = al6, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = hh[6], l = hl[6], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, hh[6] = ah6 = c & 65535 | d << 16, hl[6] = al6 = a & 65535 | b << 16, h = ah7, l = al7, a = l & 65535, b = l >>> 16, c = h & 65535, d = h >>> 16, h = hh[7], l = hl[7], a += l & 65535, b += l >>> 16, c += h & 65535, d += h >>> 16, b += a >>> 16, c += b >>> 16, d += c >>> 16, hh[7] = ah7 = c & 65535 | d << 16, hl[7] = al7 = a & 65535 | b << 16, pos += 128, n -= 128;
          }
          return n;
        }
        function crypto_hash(out, m, n) {
          var hh = new Int32Array(8), hl = new Int32Array(8), x = new Uint8Array(256), i, b = n;
          for (hh[0] = 1779033703, hh[1] = 3144134277, hh[2] = 1013904242, hh[3] = 2773480762, hh[4] = 1359893119, hh[5] = 2600822924, hh[6] = 528734635, hh[7] = 1541459225, hl[0] = 4089235720, hl[1] = 2227873595, hl[2] = 4271175723, hl[3] = 1595750129, hl[4] = 2917565137, hl[5] = 725511199, hl[6] = 4215389547, hl[7] = 327033209, crypto_hashblocks_hl(hh, hl, m, n), n %= 128, i = 0; i < n; i++) x[i] = m[b - n + i];
          for (x[n] = 128, n = 256 - 128 * (n < 112 ? 1 : 0), x[n - 9] = 0, ts64(x, n - 8, b / 536870912 | 0, b << 3), crypto_hashblocks_hl(hh, hl, x, n), i = 0; i < 8; i++) ts64(out, 8 * i, hh[i], hl[i]);
          return 0;
        }
        function add(p, q) {
          var a = gf(), b = gf(), c = gf(), d = gf(), e = gf(), f = gf(), g = gf(), h = gf(), t = gf();
          Z(a, p[1], p[0]), Z(t, q[1], q[0]), M(a, a, t), A(b, p[0], p[1]), A(t, q[0], q[1]), M(b, b, t), M(c, p[3], q[3]), M(c, c, D2), M(d, p[2], q[2]), A(d, d, d), Z(e, b, a), Z(f, d, c), A(g, d, c), A(h, b, a), M(p[0], e, f), M(p[1], h, g), M(p[2], g, f), M(p[3], e, h);
        }
        function cswap(p, q, b) {
          var i;
          for (i = 0; i < 4; i++)
            sel25519(p[i], q[i], b);
        }
        function pack(r, p) {
          var tx = gf(), ty = gf(), zi = gf();
          inv25519(zi, p[2]), M(tx, p[0], zi), M(ty, p[1], zi), pack25519(r, ty), r[31] ^= par25519(tx) << 7;
        }
        function scalarmult(p, q, s) {
          var b, i;
          for (set25519(p[0], gf0), set25519(p[1], gf1), set25519(p[2], gf1), set25519(p[3], gf0), i = 255; i >= 0; --i)
            b = s[i / 8 | 0] >> (i & 7) & 1, cswap(p, q, b), add(q, p), add(p, p), cswap(p, q, b);
        }
        function scalarbase(p, s) {
          var q = [gf(), gf(), gf(), gf()];
          set25519(q[0], X), set25519(q[1], Y), set25519(q[2], gf1), M(q[3], X, Y), scalarmult(p, q, s);
        }
        function crypto_sign_keypair(pk, sk, seeded) {
          var d = new Uint8Array(64), p = [gf(), gf(), gf(), gf()], i;
          for (seeded || randombytes(sk, 32), crypto_hash(d, sk, 32), d[0] &= 248, d[31] &= 127, d[31] |= 64, scalarbase(p, d), pack(pk, p), i = 0; i < 32; i++) sk[i + 32] = pk[i];
          return 0;
        }
        var L = new Float64Array([237, 211, 245, 92, 26, 99, 18, 88, 214, 156, 247, 162, 222, 249, 222, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 16]);
        function modL(r, x) {
          var carry, i, j, k;
          for (i = 63; i >= 32; --i) {
            for (carry = 0, j = i - 32, k = i - 12; j < k; ++j)
              x[j] += carry - 16 * x[i] * L[j - (i - 32)], carry = Math.floor((x[j] + 128) / 256), x[j] -= carry * 256;
            x[j] += carry, x[i] = 0;
          }
          for (carry = 0, j = 0; j < 32; j++)
            x[j] += carry - (x[31] >> 4) * L[j], carry = x[j] >> 8, x[j] &= 255;
          for (j = 0; j < 32; j++) x[j] -= carry * L[j];
          for (i = 0; i < 32; i++)
            x[i + 1] += x[i] >> 8, r[i] = x[i] & 255;
        }
        function reduce(r) {
          var x = new Float64Array(64), i;
          for (i = 0; i < 64; i++) x[i] = r[i];
          for (i = 0; i < 64; i++) r[i] = 0;
          modL(r, x);
        }
        function crypto_sign(sm, m, n, sk) {
          var d = new Uint8Array(64), h = new Uint8Array(64), r = new Uint8Array(64), i, j, x = new Float64Array(64), p = [gf(), gf(), gf(), gf()];
          crypto_hash(d, sk, 32), d[0] &= 248, d[31] &= 127, d[31] |= 64;
          var smlen = n + 64;
          for (i = 0; i < n; i++) sm[64 + i] = m[i];
          for (i = 0; i < 32; i++) sm[32 + i] = d[32 + i];
          for (crypto_hash(r, sm.subarray(32), n + 32), reduce(r), scalarbase(p, r), pack(sm, p), i = 32; i < 64; i++) sm[i] = sk[i];
          for (crypto_hash(h, sm, n + 64), reduce(h), i = 0; i < 64; i++) x[i] = 0;
          for (i = 0; i < 32; i++) x[i] = r[i];
          for (i = 0; i < 32; i++)
            for (j = 0; j < 32; j++)
              x[i + j] += h[i] * d[j];
          return modL(sm.subarray(32), x), smlen;
        }
        function unpackneg(r, p) {
          var t = gf(), chk = gf(), num = gf(), den = gf(), den2 = gf(), den4 = gf(), den6 = gf();
          return set25519(r[2], gf1), unpack25519(r[1], p), S(num, r[1]), M(den, num, D), Z(num, num, r[2]), A(den, r[2], den), S(den2, den), S(den4, den2), M(den6, den4, den2), M(t, den6, num), M(t, t, den), pow2523(t, t), M(t, t, num), M(t, t, den), M(t, t, den), M(r[0], t, den), S(chk, r[0]), M(chk, chk, den), neq25519(chk, num) && M(r[0], r[0], I), S(chk, r[0]), M(chk, chk, den), neq25519(chk, num) ? -1 : (par25519(r[0]) === p[31] >> 7 && Z(r[0], gf0, r[0]), M(r[3], r[0], r[1]), 0);
        }
        function crypto_sign_open(m, sm, n, pk) {
          var i, t = new Uint8Array(32), h = new Uint8Array(64), p = [gf(), gf(), gf(), gf()], q = [gf(), gf(), gf(), gf()];
          if (n < 64 || unpackneg(q, pk)) return -1;
          for (i = 0; i < n; i++) m[i] = sm[i];
          for (i = 0; i < 32; i++) m[i + 32] = pk[i];
          if (crypto_hash(h, m, n), reduce(h), scalarmult(p, q, h), scalarbase(q, sm.subarray(32)), add(p, q), pack(t, p), n -= 64, crypto_verify_32(sm, 0, t, 0)) {
            for (i = 0; i < n; i++) m[i] = 0;
            return -1;
          }
          for (i = 0; i < n; i++) m[i] = sm[i + 64];
          return n;
        }
        var crypto_secretbox_KEYBYTES = 32, crypto_secretbox_NONCEBYTES = 24, crypto_secretbox_ZEROBYTES = 32, crypto_secretbox_BOXZEROBYTES = 16, crypto_scalarmult_BYTES = 32, crypto_scalarmult_SCALARBYTES = 32, crypto_box_PUBLICKEYBYTES = 32, crypto_box_SECRETKEYBYTES = 32, crypto_box_BEFORENMBYTES = 32, crypto_box_NONCEBYTES = crypto_secretbox_NONCEBYTES, crypto_box_ZEROBYTES = crypto_secretbox_ZEROBYTES, crypto_box_BOXZEROBYTES = crypto_secretbox_BOXZEROBYTES, crypto_sign_BYTES = 64, crypto_sign_PUBLICKEYBYTES = 32, crypto_sign_SECRETKEYBYTES = 64, crypto_sign_SEEDBYTES = 32, crypto_hash_BYTES = 64;
        nacl2.lowlevel = {
          crypto_core_hsalsa20,
          crypto_stream_xor,
          crypto_stream,
          crypto_stream_salsa20_xor,
          crypto_stream_salsa20,
          crypto_onetimeauth,
          crypto_onetimeauth_verify,
          crypto_verify_16,
          crypto_verify_32,
          crypto_secretbox,
          crypto_secretbox_open,
          crypto_scalarmult,
          crypto_scalarmult_base,
          crypto_box_beforenm,
          crypto_box_afternm,
          crypto_box,
          crypto_box_open,
          crypto_box_keypair,
          crypto_hash,
          crypto_sign,
          crypto_sign_keypair,
          crypto_sign_open,
          crypto_secretbox_KEYBYTES,
          crypto_secretbox_NONCEBYTES,
          crypto_secretbox_ZEROBYTES,
          crypto_secretbox_BOXZEROBYTES,
          crypto_scalarmult_BYTES,
          crypto_scalarmult_SCALARBYTES,
          crypto_box_PUBLICKEYBYTES,
          crypto_box_SECRETKEYBYTES,
          crypto_box_BEFORENMBYTES,
          crypto_box_NONCEBYTES,
          crypto_box_ZEROBYTES,
          crypto_box_BOXZEROBYTES,
          crypto_sign_BYTES,
          crypto_sign_PUBLICKEYBYTES,
          crypto_sign_SECRETKEYBYTES,
          crypto_sign_SEEDBYTES,
          crypto_hash_BYTES,
          gf,
          D,
          L,
          pack25519,
          unpack25519,
          M,
          A,
          S,
          Z,
          pow2523,
          add,
          set25519,
          modL,
          scalarmult,
          scalarbase
        };
        function checkLengths(k, n) {
          if (k.length !== crypto_secretbox_KEYBYTES) throw new Error("bad key size");
          if (n.length !== crypto_secretbox_NONCEBYTES) throw new Error("bad nonce size");
        }
        function checkBoxLengths(pk, sk) {
          if (pk.length !== crypto_box_PUBLICKEYBYTES) throw new Error("bad public key size");
          if (sk.length !== crypto_box_SECRETKEYBYTES) throw new Error("bad secret key size");
        }
        function checkArrayTypes() {
          for (var i = 0; i < arguments.length; i++)
            if (!(arguments[i] instanceof Uint8Array))
              throw new TypeError("unexpected type, use Uint8Array");
        }
        function cleanup(arr) {
          for (var i = 0; i < arr.length; i++) arr[i] = 0;
        }
        nacl2.randomBytes = function(n) {
          var b = new Uint8Array(n);
          return randombytes(b, n), b;
        }, nacl2.secretbox = function(msg, nonce, key) {
          checkArrayTypes(msg, nonce, key), checkLengths(key, nonce);
          for (var m = new Uint8Array(crypto_secretbox_ZEROBYTES + msg.length), c = new Uint8Array(m.length), i = 0; i < msg.length; i++) m[i + crypto_secretbox_ZEROBYTES] = msg[i];
          return crypto_secretbox(c, m, m.length, nonce, key), c.subarray(crypto_secretbox_BOXZEROBYTES);
        }, nacl2.secretbox.open = function(box, nonce, key) {
          checkArrayTypes(box, nonce, key), checkLengths(key, nonce);
          for (var c = new Uint8Array(crypto_secretbox_BOXZEROBYTES + box.length), m = new Uint8Array(c.length), i = 0; i < box.length; i++) c[i + crypto_secretbox_BOXZEROBYTES] = box[i];
          return c.length < 32 || crypto_secretbox_open(m, c, c.length, nonce, key) !== 0 ? null : m.subarray(crypto_secretbox_ZEROBYTES);
        }, nacl2.secretbox.keyLength = crypto_secretbox_KEYBYTES, nacl2.secretbox.nonceLength = crypto_secretbox_NONCEBYTES, nacl2.secretbox.overheadLength = crypto_secretbox_BOXZEROBYTES, nacl2.scalarMult = function(n, p) {
          if (checkArrayTypes(n, p), n.length !== crypto_scalarmult_SCALARBYTES) throw new Error("bad n size");
          if (p.length !== crypto_scalarmult_BYTES) throw new Error("bad p size");
          var q = new Uint8Array(crypto_scalarmult_BYTES);
          return crypto_scalarmult(q, n, p), q;
        }, nacl2.scalarMult.base = function(n) {
          if (checkArrayTypes(n), n.length !== crypto_scalarmult_SCALARBYTES) throw new Error("bad n size");
          var q = new Uint8Array(crypto_scalarmult_BYTES);
          return crypto_scalarmult_base(q, n), q;
        }, nacl2.scalarMult.scalarLength = crypto_scalarmult_SCALARBYTES, nacl2.scalarMult.groupElementLength = crypto_scalarmult_BYTES, nacl2.box = function(msg, nonce, publicKey, secretKey) {
          var k = nacl2.box.before(publicKey, secretKey);
          return nacl2.secretbox(msg, nonce, k);
        }, nacl2.box.before = function(publicKey, secretKey) {
          checkArrayTypes(publicKey, secretKey), checkBoxLengths(publicKey, secretKey);
          var k = new Uint8Array(crypto_box_BEFORENMBYTES);
          return crypto_box_beforenm(k, publicKey, secretKey), k;
        }, nacl2.box.after = nacl2.secretbox, nacl2.box.open = function(msg, nonce, publicKey, secretKey) {
          var k = nacl2.box.before(publicKey, secretKey);
          return nacl2.secretbox.open(msg, nonce, k);
        }, nacl2.box.open.after = nacl2.secretbox.open, nacl2.box.keyPair = function() {
          var pk = new Uint8Array(crypto_box_PUBLICKEYBYTES), sk = new Uint8Array(crypto_box_SECRETKEYBYTES);
          return crypto_box_keypair(pk, sk), { publicKey: pk, secretKey: sk };
        }, nacl2.box.keyPair.fromSecretKey = function(secretKey) {
          if (checkArrayTypes(secretKey), secretKey.length !== crypto_box_SECRETKEYBYTES)
            throw new Error("bad secret key size");
          var pk = new Uint8Array(crypto_box_PUBLICKEYBYTES);
          return crypto_scalarmult_base(pk, secretKey), { publicKey: pk, secretKey: new Uint8Array(secretKey) };
        }, nacl2.box.publicKeyLength = crypto_box_PUBLICKEYBYTES, nacl2.box.secretKeyLength = crypto_box_SECRETKEYBYTES, nacl2.box.sharedKeyLength = crypto_box_BEFORENMBYTES, nacl2.box.nonceLength = crypto_box_NONCEBYTES, nacl2.box.overheadLength = nacl2.secretbox.overheadLength, nacl2.sign = function(msg, secretKey) {
          if (checkArrayTypes(msg, secretKey), secretKey.length !== crypto_sign_SECRETKEYBYTES)
            throw new Error("bad secret key size");
          var signedMsg = new Uint8Array(crypto_sign_BYTES + msg.length);
          return crypto_sign(signedMsg, msg, msg.length, secretKey), signedMsg;
        }, nacl2.sign.open = function(signedMsg, publicKey) {
          if (checkArrayTypes(signedMsg, publicKey), publicKey.length !== crypto_sign_PUBLICKEYBYTES)
            throw new Error("bad public key size");
          var tmp = new Uint8Array(signedMsg.length), mlen = crypto_sign_open(tmp, signedMsg, signedMsg.length, publicKey);
          if (mlen < 0) return null;
          for (var m = new Uint8Array(mlen), i = 0; i < m.length; i++) m[i] = tmp[i];
          return m;
        }, nacl2.sign.detached = function(msg, secretKey) {
          for (var signedMsg = nacl2.sign(msg, secretKey), sig = new Uint8Array(crypto_sign_BYTES), i = 0; i < sig.length; i++) sig[i] = signedMsg[i];
          return sig;
        }, nacl2.sign.detached.verify = function(msg, sig, publicKey) {
          if (checkArrayTypes(msg, sig, publicKey), sig.length !== crypto_sign_BYTES)
            throw new Error("bad signature size");
          if (publicKey.length !== crypto_sign_PUBLICKEYBYTES)
            throw new Error("bad public key size");
          var sm = new Uint8Array(crypto_sign_BYTES + msg.length), m = new Uint8Array(crypto_sign_BYTES + msg.length), i;
          for (i = 0; i < crypto_sign_BYTES; i++) sm[i] = sig[i];
          for (i = 0; i < msg.length; i++) sm[i + crypto_sign_BYTES] = msg[i];
          return crypto_sign_open(m, sm, sm.length, publicKey) >= 0;
        }, nacl2.sign.keyPair = function() {
          var pk = new Uint8Array(crypto_sign_PUBLICKEYBYTES), sk = new Uint8Array(crypto_sign_SECRETKEYBYTES);
          return crypto_sign_keypair(pk, sk), { publicKey: pk, secretKey: sk };
        }, nacl2.sign.keyPair.fromSecretKey = function(secretKey) {
          if (checkArrayTypes(secretKey), secretKey.length !== crypto_sign_SECRETKEYBYTES)
            throw new Error("bad secret key size");
          for (var pk = new Uint8Array(crypto_sign_PUBLICKEYBYTES), i = 0; i < pk.length; i++) pk[i] = secretKey[32 + i];
          return { publicKey: pk, secretKey: new Uint8Array(secretKey) };
        }, nacl2.sign.keyPair.fromSeed = function(seed) {
          if (checkArrayTypes(seed), seed.length !== crypto_sign_SEEDBYTES)
            throw new Error("bad seed size");
          for (var pk = new Uint8Array(crypto_sign_PUBLICKEYBYTES), sk = new Uint8Array(crypto_sign_SECRETKEYBYTES), i = 0; i < 32; i++) sk[i] = seed[i];
          return crypto_sign_keypair(pk, sk, !0), { publicKey: pk, secretKey: sk };
        }, nacl2.sign.publicKeyLength = crypto_sign_PUBLICKEYBYTES, nacl2.sign.secretKeyLength = crypto_sign_SECRETKEYBYTES, nacl2.sign.seedLength = crypto_sign_SEEDBYTES, nacl2.sign.signatureLength = crypto_sign_BYTES, nacl2.hash = function(msg) {
          checkArrayTypes(msg);
          var h = new Uint8Array(crypto_hash_BYTES);
          return crypto_hash(h, msg, msg.length), h;
        }, nacl2.hash.hashLength = crypto_hash_BYTES, nacl2.verify = function(x, y) {
          return checkArrayTypes(x, y), x.length === 0 || y.length === 0 || x.length !== y.length ? !1 : vn(x, 0, y, 0, x.length) === 0;
        }, nacl2.setPRNG = function(fn) {
          randombytes = fn;
        };
      })(typeof module != "undefined" && module.exports ? module.exports : self.nacl = self.nacl || {});
    }
  });

  // src/commerce.ts
  var commerce_exports = {};
  __export(commerce_exports, {
    createMiniGameRuntime: () => createMiniGameRuntime,
    createXiaohongshuPlatform: () => createXiaohongshuPlatform
  });

  // src/runtime.ts
  var import_jsqr = __toESM(require_jsQR());

  // src/index.ts
  var import_sha256 = __toESM(require_sha256()), import_tweetnacl = __toESM(require_nacl_fast()), PAYLOAD_LENGTH = 41;
  var TOKEN_LENGTH = 140, BASE64URL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", INVALID_BASE64URL_PATTERN = /[^A-Za-z0-9_-]/, UUID_PATTERN = /^(?:[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}|00000000-0000-0000-0000-000000000000|ffffffff-ffff-ffff-ffff-ffffffffffff)$/;
  var INSTALLATION_DOMAIN = asciiBytes("ViceMe.MiniGame.Installation.v1\0"), LICENSE_DOMAIN = asciiBytes("ViceMe.MiniGame.License.v1\0"), ED25519_ORDER = hexBytes(
    "edd3f55c1a631258d69cf7a2def9de1400000000000000000000000000000010"
  );
  function installationDigest(installationId) {
    let message = new Uint8Array(INSTALLATION_DOMAIN.length + 16);
    return message.set(INSTALLATION_DOMAIN), message.set(uuidBytes(installationId), INSTALLATION_DOMAIN.length), (0, import_sha256.hash)(message).slice(0, 16);
  }
  function licenseSigningMessage(workId, payload) {
    assertPayload(payload);
    let message = new Uint8Array(LICENSE_DOMAIN.length + 16 + PAYLOAD_LENGTH);
    return message.set(LICENSE_DOMAIN), message.set(uuidBytes(workId), LICENSE_DOMAIN.length), message.set(payload, LICENSE_DOMAIN.length + 16), message;
  }
  function verifyLicense(token, configuration) {
    try {
      let bytes = decodeBase64Url(token, TOKEN_LENGTH), payload = bytes.subarray(0, PAYLOAD_LENGTH);
      if (payload[0] !== environmentByte(configuration.environment) || !equalBytes(payload.subarray(1, 17), uuidBytes(configuration.itemId)) || !equalBytes(
        payload.subarray(17, 33),
        installationDigest(configuration.installationId)
      ))
        return invalidLicense();
      let signature = bytes.subarray(PAYLOAD_LENGTH), publicKey = decodeBase64Url(configuration.publicKey, 43);
      return !isCanonicalSignature(signature) || !import_tweetnacl.default.sign.detached.verify(
        licenseSigningMessage(configuration.workId, payload),
        signature,
        publicKey
      ) ? invalidLicense() : { valid: !0, licenseId: bytesHex(payload.subarray(33, 41)) };
    } catch (e) {
      return invalidLicense();
    }
  }
  function invalidLicense() {
    return { valid: !1, code: "MINI_GAME_LICENSE_INVALID" };
  }
  function environmentByte(environment) {
    if (environment === "SANDBOX") return 16;
    if (environment === "PRODUCTION") return 17;
    throw new TypeError("Invalid license environment");
  }
  function assertPayload(payload) {
    if (!(payload instanceof Uint8Array) || payload.length !== PAYLOAD_LENGTH || payload[0] !== 16 && payload[0] !== 17)
      throw new TypeError("Invalid license payload");
    let item = payload.subarray(1, 17);
    if (!item.every((byte) => byte === 0) && !item.every((byte) => byte === 255) && (item[6] >> 4 < 1 || item[6] >> 4 > 8 || (item[8] & 192) !== 128))
      throw new TypeError("Invalid license item identifier");
  }
  function uuidBytes(value) {
    if (typeof value != "string" || value.length !== 36 || !UUID_PATTERN.test(value))
      throw new TypeError("Invalid license UUID");
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
  function bytesHex(bytes) {
    let value = "";
    for (let byte of bytes) value += byte.toString(16).padStart(2, "0");
    return value;
  }
  function equalBytes(left, right) {
    for (let index = 0; index < left.length; index++)
      if (left[index] !== right[index]) return !1;
    return !0;
  }
  function isCanonicalSignature(signature) {
    for (let index = 31; index >= 0; index--) {
      if (signature[index + 32] < ED25519_ORDER[index]) return !0;
      if (signature[index + 32] > ED25519_ORDER[index]) return !1;
    }
    return !1;
  }
  function decodeBase64Url(value, length) {
    if (typeof value != "string" || value.length !== length || INVALID_BASE64URL_PATTERN.test(value))
      throw new TypeError("Invalid license Base64URL");
    let bytes = new Uint8Array(Math.floor(length * 6 / 8)), bits = 0, available = 0, offset = 0;
    for (let index = 0; index < length; index++)
      bits = bits << 6 | BASE64URL.indexOf(value[index]), available += 6, available >= 8 && (available -= 8, bytes[offset++] = bits >> available & 255);
    if ((bits & (1 << available) - 1) !== 0)
      throw new TypeError("Noncanonical license Base64URL");
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
    INSTALLATION_CORRUPT: "\u8D2D\u4E70\u8BB0\u5F55\u635F\u574F\u6216\u7248\u672C\u4E0D\u517C\u5BB9\uFF0C\u8BF7\u8054\u7CFB\u6E38\u620F\u4F5C\u8005\uFF0C\u4E0D\u8981\u6E05\u9664\u6E38\u620F\u6570\u636E\u3002",
    NOT_INITIALIZED: "\u8D2D\u4E70\u529F\u80FD\u5C1A\u672A\u5C31\u7EEA\uFF0C\u8BF7\u5148\u91CD\u65B0\u521D\u59CB\u5316\uFF1B\u666E\u901A\u6E38\u620F\u4E0D\u53D7\u5F71\u54CD\u3002",
    ITEM_NOT_FOUND: "\u6E38\u620F\u672A\u914D\u7F6E\u6B64\u9053\u5177\uFF0C\u8BF7\u8054\u7CFB\u6E38\u620F\u4F5C\u8005\u66F4\u65B0\u3002",
    RANDOM_UNAVAILABLE: "\u65E0\u6CD5\u521B\u5EFA\u5B89\u5168\u7684\u8D2D\u4E70\u5B9E\u4F8B\uFF0C\u8BF7\u91CD\u65B0\u6253\u5F00\u6E38\u620F\u540E\u91CD\u8BD5\u3002",
    CARD_SAVE_FAILED: "\u4ED8\u6B3E\u5361\u672A\u80FD\u4FDD\u5B58\uFF0C\u8BF7\u5141\u8BB8\u4FDD\u5B58\u5230\u76F8\u518C\u540E\u91CD\u8BD5\u3002",
    MINI_GAME_LICENSE_INVALID: "\u6CE8\u518C\u7801\u65E0\u6548\u6216\u4E0D\u5C5E\u4E8E\u5F53\u524D\u6E38\u620F\u3001\u9053\u5177\u3001\u73AF\u5883\u6216\u5B89\u88C5\u5B9E\u4F8B\uFF0C\u8BF7\u4ECE\u672C\u6E38\u620F\u4ED8\u6B3E\u5361\u91CD\u65B0\u83B7\u53D6\u3002",
    IMAGE_CANCELLED: "\u5DF2\u53D6\u6D88\u9009\u62E9\u56FE\u7247\uFF0C\u53EF\u91CD\u65B0\u9009\u62E9\u5151\u6362\u4E8C\u7EF4\u7801\u6216\u8F93\u5165\u5B8C\u6574\u6CE8\u518C\u7801\u3002",
    IMAGE_NO_QR: "\u56FE\u7247\u4E2D\u672A\u627E\u5230\u53EF\u8BC6\u522B\u7684\u4E8C\u7EF4\u7801\uFF0C\u8BF7\u9009\u62E9\u6E05\u6670\u5B8C\u6574\u7684\u5151\u6362\u4E8C\u7EF4\u7801\u3002",
    IMAGE_UNSUPPORTED: "\u65E0\u6CD5\u8BFB\u53D6\u6B64\u56FE\u7247\u683C\u5F0F\uFF0C\u8BF7\u9009\u62E9 PNG \u6216 JPEG \u5151\u6362\u4E8C\u7EF4\u7801\u3002",
    IMAGE_TOO_LARGE: "\u56FE\u7247\u8FC7\u5927\uFF0C\u8BF7\u9009\u62E9\u8F83\u5C0F\u7684\u5151\u6362\u4E8C\u7EF4\u7801\u56FE\u7247\u3002",
    IMAGE_READ_FAILED: "\u56FE\u7247\u8BFB\u53D6\u5931\u8D25\uFF0C\u8BF7\u91CD\u65B0\u9009\u62E9\u6E05\u6670\u5B8C\u6574\u7684\u5151\u6362\u4E8C\u7EF4\u7801\u3002",
    IMAGE_SELECTION_BUSY: "\u56FE\u7247\u9009\u62E9\u5C1A\u672A\u7ED3\u675F\uFF0C\u8BF7\u5B8C\u6210\u5F53\u524D\u9009\u62E9\u540E\u91CD\u8BD5\u3002",
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
      "workId",
      "workTitle",
      "environment",
      "publicClientId",
      "publicKey",
      "checkoutOrigin",
      "items"
    ]) || !isUuid(value.workId) || !nonempty(value.workTitle) || value.environment !== "SANDBOX" && value.environment !== "PRODUCTION" || typeof value.publicClientId != "string" || value.publicClientId.length !== 36 || !/^vca_[A-Za-z0-9_-]{32}$/.test(value.publicClientId) || typeof value.publicKey != "string" || value.publicKey.length !== 43 || !/^[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]$/.test(value.publicKey) || typeof value.checkoutOrigin != "string" || !Array.isArray(value.items))
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
        "licenses"
      ]) || value.version !== 1 || value.workId !== config.workId || value.environment !== config.environment || value.publicClientId !== config.publicClientId || !isUuid(value.installationId) || !isObject(value.licenses))
        return null;
      let licenses = {};
      for (let itemId of Object.keys(value.licenses)) {
        let token = value.licenses[itemId];
        if (!isUuid(itemId) || typeof token != "string" || !verifyLicense(token, {
          workId: config.workId,
          environment: config.environment,
          publicKey: config.publicKey,
          itemId,
          installationId: value.installationId
        }).valid)
          return null;
        licenses[itemId] = token;
      }
      return {
        version: 1,
        workId: config.workId,
        environment: config.environment,
        publicClientId: config.publicClientId,
        installationId: value.installationId,
        licenses
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
      return !restored || restored.installationId !== next.installationId || Object.keys(restored.licenses).length !== Object.keys(next.licenses).length || Object.keys(next.licenses).some(
        (id) => restored.licenses[id] !== next.licenses[id]
      ) ? !1 : (record = restored, !0);
    }
    async function redeem(alias, token) {
      if (!record) return failure("NOT_INITIALIZED");
      let item = items.get(alias);
      if (!item) return failure("ITEM_NOT_FOUND");
      let result = verifyLicense(token, {
        workId: config.workId,
        environment: config.environment,
        publicKey: config.publicKey,
        itemId: item.id,
        installationId: record.installationId
      });
      if (!result.valid) return failure(result.code);
      if (record.licenses[item.id] !== token) {
        let next = __spreadProps(__spreadValues({}, record), {
          licenses: __spreadProps(__spreadValues({}, record.licenses), { [item.id]: token })
        });
        if (!await persist(next)) return failure("STORAGE_READBACK_FAILED");
      }
      return { ok: !0, unlocked: !0, licenseId: result.licenseId };
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
              version: 1,
              workId: config.workId,
              environment: config.environment,
              publicClientId: config.publicClientId,
              installationId: pendingInstallationId,
              licenses: {}
            };
          } else {
            observedInstallation = !0;
            let restored = parseRecord(stored);
            if (!restored || pendingInstallationId && pendingInstallationId !== restored.installationId)
              return failure("INSTALLATION_CORRUPT");
            next = restored;
          }
          return await persist(next) ? (observedInstallation = !0, pendingInstallationId = next.installationId, { ok: !0, installationId: next.installationId }) : failure("STORAGE_READBACK_FAILED");
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
          let checkoutUrl = "".concat(config.checkoutOrigin, "/checkout/mini-game?v=1") + "&publicClientId=".concat(config.publicClientId, "&itemId=").concat(item.id) + "&installationId=".concat(record.installationId, "&runtimeVersion=1.0.0&launchId=").concat(launchId);
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
      redeemLicense(alias, token) {
        return serialize(() => redeem(alias, token));
      },
      async importLicenseImage(alias) {
        if (!config) return failure("CONFIGURATION_INVALID");
        try {
          if (!record) return failure("NOT_INITIALIZED");
          if (!items.has(alias)) return failure("ITEM_NOT_FOUND");
          let image = await storage(
            () => platform.selectImage(),
            "IMAGE_READ_FAILED"
          );
          if (image === null) return failure("IMAGE_CANCELLED");
          if (!isObject(image) || !Number.isSafeInteger(image.width) || !Number.isSafeInteger(image.height) || image.width <= 0 || image.height <= 0 || Object.prototype.toString.call(image.data) !== "[object Uint8ClampedArray]" || image.data.length !== image.width * image.height * 4)
            return failure("IMAGE_READ_FAILED");
          if (image.width * image.height > 16e6)
            return failure("IMAGE_TOO_LARGE");
          let decoded;
          try {
            decoded = (0, import_jsqr.default)(image.data, image.width, image.height);
          } catch (e) {
            return failure("IMAGE_READ_FAILED");
          }
          return decoded ? serialize(() => redeem(alias, decoded.data)) : failure("IMAGE_NO_QR");
        } catch (error) {
          return failure(
            isObject(error) && typeof error.code == "string" && Object.prototype.hasOwnProperty.call(MESSAGES, error.code) ? error.code : "IMAGE_READ_FAILED"
          );
        }
      },
      isUnlocked(alias) {
        return serialize(async () => {
          if (!record) return failure("NOT_INITIALIZED");
          let item = items.get(alias);
          return item ? {
            ok: !0,
            unlocked: Object.prototype.hasOwnProperty.call(
              record.licenses,
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
  var MAX_IMAGE_BYTES = 12 * 1024 * 1024, MAX_IMAGE_PIXELS = 12 * 1024 * 1024, MAX_IMAGE_SIDE = 8192, MAX_DECODE_SIDE = 2048;
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
    let host = window, selectingImage = !1;
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
            "\u8D2D\u4E70\u8BB0\u5F55\u4FDD\u5B58\u5931\u8D25\uFF0C\u8BF7\u68C0\u67E5\u5B58\u50A8\u7A7A\u95F4\u540E\u91CD\u8BD5\u3002\u8BF7\u4FDD\u7559\u6CE8\u518C\u7801\u3002"
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
      },
      async selectImage() {
        if (selectingImage)
          throw failure2("IMAGE_SELECTION_BUSY", "\u8BF7\u5148\u5B8C\u6210\u5F53\u524D\u56FE\u7247\u9009\u62E9\uFF0C\u518D\u91CD\u8BD5\u3002");
        selectingImage = !0;
        try {
          return await selectLocalImage(host);
        } finally {
          selectingImage = !1;
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
    ), context.font = "24px sans-serif", context.fillText("\u6743\u76CA\u7ED1\u5B9A\u5F53\u524D\u6E38\u620F\u5B89\u88C5\uFF0C\u8BA2\u5355\u5C5E\u4E8E\u4ED8\u6B3E\u8D26\u53F7", 450, 1084), context.fillText("\u5B8C\u6210\u540E\u8FD4\u56DE\u6E38\u620F\uFF0C\u8F93\u5165\u6CE8\u518C\u7801\u6216\u5BFC\u5165\u5151\u6362\u4E8C\u7EF4\u7801", 450, 1130), canvas;
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
  function selectLocalImage(window) {
    return new Promise((resolve, reject) => {
      let document = window.document, input = document.createElement("input");
      input.type = "file", input.accept = "image/png,image/jpeg,image/webp,image/gif", input.style.cssText = "position:fixed;left:-10000px;width:1px;height:1px;opacity:0";
      let finished = !1, reading = !1, armed = !1, cancelTimer, armTimer, reader, image, canvas, objectUrl, finish = (result, error) => {
        finished || (finished = !0, window.clearTimeout(cancelTimer), window.clearTimeout(armTimer), input.removeEventListener("change", change), input.removeEventListener("cancel", cancel), window.removeEventListener("focus", focus), document.removeEventListener("visibilitychange", visibility), input.parentNode && input.parentNode.removeChild(input), reader && (reader.onload = null, reader.onerror = null, reader.onabort = null, reader.readyState === 1 && reader.abort()), image && (image.onload = null, image.onerror = null, image.removeAttribute("src")), objectUrl && window.URL.revokeObjectURL(objectUrl), canvas && (canvas.width = 0, canvas.height = 0), error ? reject(error) : resolve(result));
      }, failRead = () => finish(
        null,
        failure2(
          "IMAGE_READ_FAILED",
          "\u65E0\u6CD5\u8BFB\u53D6\u8FD9\u5F20\u56FE\u7247\uFF0C\u8BF7\u91CD\u65B0\u9009\u62E9\u6E05\u6670\u7684\u5151\u6362\u4E8C\u7EF4\u7801\u56FE\u7247\uFF0C\u6216\u76F4\u63A5\u8F93\u5165\u6CE8\u518C\u7801\u3002"
        )
      ), cancel = () => {
        reading || finish(null);
      }, change = () => {
        if (finished || reading) return;
        let file = input.files && input.files[0];
        if (!file) {
          finish(null);
          return;
        }
        if (reading = !0, window.clearTimeout(cancelTimer), !file.size || file.size > MAX_IMAGE_BYTES) {
          finish(
            null,
            failure2(
              "IMAGE_TOO_LARGE",
              "\u56FE\u7247\u8FC7\u5927\u6216\u4E3A\u7A7A\uFF0C\u8BF7\u9009\u62E9 12 MB \u4EE5\u5185\u7684\u5151\u6362\u4E8C\u7EF4\u7801\u56FE\u7247\u3002"
            )
          );
          return;
        }
        try {
          reader = new window.FileReader(), reader.onerror = failRead, reader.onabort = failRead, reader.onload = () => {
            try {
              if (!reader || !reader.result || typeof reader.result == "string") {
                failRead();
                return;
              }
              let dimensions = imageDimensions(new Uint8Array(reader.result));
              if (!dimensions) {
                finish(
                  null,
                  failure2(
                    "IMAGE_UNSUPPORTED",
                    "\u8BF7\u9009\u62E9 PNG\u3001JPEG\u3001WebP \u6216 GIF \u683C\u5F0F\u7684\u5151\u6362\u4E8C\u7EF4\u7801\u56FE\u7247\uFF0C\u6216\u76F4\u63A5\u8F93\u5165\u6CE8\u518C\u7801\u3002"
                  )
                );
                return;
              }
              let [width, height] = dimensions;
              if (!validDimensions(width, height)) {
                finish(
                  null,
                  failure2(
                    "IMAGE_TOO_LARGE",
                    "\u56FE\u7247\u5C3A\u5BF8\u8FC7\u5927\uFF0C\u8BF7\u88C1\u526A\u5230\u5151\u6362\u4E8C\u7EF4\u7801\u533A\u57DF\u540E\u91CD\u8BD5\u3002"
                  )
                );
                return;
              }
              image = document.createElement("img"), image.onerror = failRead, image.onload = () => {
                try {
                  if (!image || !validDimensions(image.naturalWidth, image.naturalHeight)) {
                    failRead();
                    return;
                  }
                  let scale = Math.min(
                    1,
                    MAX_DECODE_SIDE / Math.max(image.naturalWidth, image.naturalHeight)
                  ), width2 = Math.max(
                    1,
                    Math.round(image.naturalWidth * scale)
                  ), height2 = Math.max(
                    1,
                    Math.round(image.naturalHeight * scale)
                  );
                  canvas = document.createElement("canvas"), canvas.width = width2, canvas.height = height2;
                  let context = canvas.getContext("2d");
                  if (!context) {
                    failRead();
                    return;
                  }
                  context.fillStyle = "#ffffff", context.fillRect(0, 0, width2, height2), context.drawImage(image, 0, 0, width2, height2), finish({
                    data: context.getImageData(0, 0, width2, height2).data,
                    width: width2,
                    height: height2
                  });
                } catch (e) {
                  failRead();
                }
              }, objectUrl = window.URL.createObjectURL(file), image.src = objectUrl;
            } catch (e) {
              failRead();
            }
          }, reader.readAsArrayBuffer(file.slice(0, 512 * 1024));
        } catch (e) {
          failRead();
        }
      }, focus = () => {
        !armed || reading || finished || document.visibilityState === "hidden" || (window.clearTimeout(cancelTimer), cancelTimer = window.setTimeout(() => {
          finished || reading || document.visibilityState === "hidden" || (input.files && input.files.length ? change() : finish(null));
        }, 1500));
      }, visibility = () => {
        document.visibilityState === "visible" && focus();
      };
      input.addEventListener("change", change), input.addEventListener("cancel", cancel), window.addEventListener("focus", focus), document.addEventListener("visibilitychange", visibility);
      try {
        (document.body || document.documentElement).appendChild(input), armTimer = window.setTimeout(() => {
          armed = !0;
        }, 0), input.click();
      } catch (e) {
        failRead();
      }
    });
  }
  function validDimensions(width, height) {
    return width > 0 && height > 0 && width <= MAX_IMAGE_SIDE && height <= MAX_IMAGE_SIDE && width * height <= MAX_IMAGE_PIXELS;
  }
  function imageDimensions(bytes) {
    if (bytes.length < 24) return null;
    let view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    if (view.getUint32(0) === 2303741511 && view.getUint32(4) === 218765834 && view.getUint32(12) === 1229472850)
      return [view.getUint32(16), view.getUint32(20)];
    if (view.getUint32(0) === 1195984440 && (view.getUint16(4) === 14177 || view.getUint16(4) === 14689))
      return [view.getUint16(6, !0), view.getUint16(8, !0)];
    if (view.getUint16(0) === 65496) {
      let offset = 2;
      for (; offset + 4 <= bytes.length; ) {
        if (bytes[offset++] !== 255) return null;
        for (; bytes[offset] === 255; ) offset++;
        let marker = bytes[offset++];
        if (marker === void 0 || marker === 218 || marker === 217)
          return null;
        if (marker === 1 || marker >= 208 && marker <= 215) continue;
        if (offset + 2 > bytes.length) return null;
        let length = view.getUint16(offset);
        if (length < 2 || offset + length > bytes.length) return null;
        if (marker >= 192 && marker <= 207 && marker !== 196 && marker !== 200 && marker !== 204)
          return length < 8 ? null : [view.getUint16(offset + 5), view.getUint16(offset + 3)];
        offset += length;
      }
      return null;
    }
    if (view.getUint32(0) === 1380533830 && view.getUint32(8) === 1464156752 && bytes.length >= 30) {
      let kind = view.getUint32(12);
      if (kind === 1448097880)
        return [
          1 + bytes[24] + (bytes[25] << 8) + (bytes[26] << 16),
          1 + bytes[27] + (bytes[28] << 8) + (bytes[29] << 16)
        ];
      if (kind === 1448097824 && bytes[23] === 157 && bytes[24] === 1 && bytes[25] === 42)
        return [
          view.getUint16(26, !0) & 16383,
          view.getUint16(28, !0) & 16383
        ];
      if (kind === 1448097868 && bytes[20] === 47) {
        let bits = view.getUint32(21, !0);
        return [(bits & 16383) + 1, (bits >>> 14 & 16383) + 1];
      }
    }
    return null;
  }
  return __toCommonJS(commerce_exports);
})();
