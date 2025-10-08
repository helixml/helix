export const U8_MAX = 255;
export const U16_MAX = 65535;
export const I16_MAX = 32767;
export class ByteBuffer {
    constructor(value, littleEndian) {
        this.position = 0;
        this.limit = 0;
        this.littleEndian = littleEndian !== null && littleEndian !== void 0 ? littleEndian : false;
        if (value instanceof Uint8Array) {
            this.buffer = value;
        }
        else {
            this.buffer = new Uint8Array(value !== null && value !== void 0 ? value : 0);
        }
    }
    bytesUsed(amount, reading) {
        this.position += amount;
        if (reading && this.position > this.limit) {
            throw "failed to read over the limit";
        }
    }
    putU8Array(data) {
        this.buffer.set(data, this.position);
        this.bytesUsed(data.length, false);
    }
    putU8(data) {
        const view = new DataView(this.buffer.buffer);
        view.setUint8(this.position, data);
        this.bytesUsed(1, false);
    }
    putBool(data) {
        this.putU8(data ? 1 : 0);
    }
    putI8(data) {
        const view = new DataView(this.buffer.buffer);
        view.setInt8(this.position, data);
        this.bytesUsed(1, false);
    }
    putU16(data) {
        const view = new DataView(this.buffer.buffer);
        view.setUint16(this.position, data, this.littleEndian);
        this.bytesUsed(2, false);
    }
    putI16(data) {
        const view = new DataView(this.buffer.buffer);
        view.setInt16(this.position, data, this.littleEndian);
        this.bytesUsed(2, false);
    }
    putU32(data) {
        const view = new DataView(this.buffer.buffer);
        view.setUint32(this.position, data, this.littleEndian);
        this.bytesUsed(4, false);
    }
    putUtf8(text) {
        const encoder = new TextEncoder();
        const result = encoder.encodeInto(text, this.buffer.subarray(this.position));
        this.bytesUsed(result.written, false);
        if (result.read != text.length) {
            throw "failed to put utf8 text";
        }
    }
    putF32(data) {
        const view = new DataView(this.buffer.buffer);
        view.setFloat32(this.position, data, this.littleEndian);
        this.bytesUsed(4, false);
    }
    get(buffer, offset, length) {
        buffer.set(this.buffer.subarray(this.position, this.position + length), offset);
        this.bytesUsed(length, true);
    }
    getU8() {
        const view = new DataView(this.buffer.buffer);
        const byte = view.getUint8(this.position);
        this.bytesUsed(1, true);
        return byte;
    }
    getU16() {
        const view = new DataView(this.buffer.buffer);
        const byte = view.getUint16(this.position);
        this.bytesUsed(2, true);
        return byte;
    }
    getBool() {
        return this.getU8() != 0;
    }
    reset() {
        this.position = 0;
        this.limit = 0;
    }
    flip() {
        this.limit = this.position;
        this.position = 0;
    }
    isLittleEndian() {
        return this.littleEndian;
    }
    getPosition() {
        return this.position;
    }
    getReadBuffer() {
        return this.buffer.slice(0, this.limit);
    }
}
