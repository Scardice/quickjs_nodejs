package buffer

import (
	"encoding/binary"
	"math"
	"math/big"

	quickjs "github.com/buke/quickjs-go"
)

type numericMethod func(*quickjs.Context, *quickjs.Value, []*quickjs.Value) *quickjs.Value

var numericMethods = map[string]numericMethod{
	"readBigInt64BE":   readBigInt64BE,
	"readBigInt64LE":   readBigInt64LE,
	"readBigUInt64BE":  readBigUInt64BE,
	"readBigUInt64LE":  readBigUInt64LE,
	"readBigUint64BE":  readBigUInt64BE,
	"readBigUint64LE":  readBigUInt64LE,
	"readDoubleBE":     readDoubleBE,
	"readDoubleLE":     readDoubleLE,
	"readFloatBE":      readFloatBE,
	"readFloatLE":      readFloatLE,
	"readInt8":         readInt8,
	"readInt16BE":      readInt16BE,
	"readInt16LE":      readInt16LE,
	"readInt32BE":      readInt32BE,
	"readInt32LE":      readInt32LE,
	"readIntBE":        readIntBE,
	"readIntLE":        readIntLE,
	"readUInt8":        readUInt8,
	"readUInt16BE":     readUInt16BE,
	"readUInt16LE":     readUInt16LE,
	"readUInt32BE":     readUInt32BE,
	"readUInt32LE":     readUInt32LE,
	"readUIntBE":       readUIntBE,
	"readUIntLE":       readUIntLE,
	"readUint8":        readUInt8,
	"readUint16BE":     readUInt16BE,
	"readUint16LE":     readUInt16LE,
	"readUint32BE":     readUInt32BE,
	"readUint32LE":     readUInt32LE,
	"readUintBE":       readUIntBE,
	"readUintLE":       readUIntLE,
	"writeBigInt64BE":  writeBigInt64BE,
	"writeBigInt64LE":  writeBigInt64LE,
	"writeBigUInt64BE": writeBigUInt64BE,
	"writeBigUInt64LE": writeBigUInt64LE,
	"writeBigUint64BE": writeBigUInt64BE,
	"writeBigUint64LE": writeBigUInt64LE,
	"writeDoubleBE":    writeDoubleBE,
	"writeDoubleLE":    writeDoubleLE,
	"writeFloatBE":     writeFloatBE,
	"writeFloatLE":     writeFloatLE,
	"writeInt8":        writeInt8,
	"writeInt16BE":     writeInt16BE,
	"writeInt16LE":     writeInt16LE,
	"writeInt32BE":     writeInt32BE,
	"writeInt32LE":     writeInt32LE,
	"writeIntBE":       writeIntBE,
	"writeIntLE":       writeIntLE,
	"writeUInt8":       writeUInt8,
	"writeUInt16BE":    writeUInt16BE,
	"writeUInt16LE":    writeUInt16LE,
	"writeUInt32BE":    writeUInt32BE,
	"writeUInt32LE":    writeUInt32LE,
	"writeUIntBE":      writeUIntBE,
	"writeUIntLE":      writeUIntLE,
	"writeUint8":       writeUInt8,
	"writeUint16BE":    writeUInt16BE,
	"writeUint16LE":    writeUInt16LE,
	"writeUint32BE":    writeUInt32BE,
	"writeUint32LE":    writeUInt32LE,
	"writeUintBE":      writeUIntBE,
	"writeUintLE":      writeUIntLE,
}

func readBigInt64BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 8)
	if !ok {
		return failure
	}
	return ctx.NewBigInt64(int64(binary.BigEndian.Uint64(data[offset : offset+8])))
}

func readBigInt64LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 8)
	if !ok {
		return failure
	}
	return ctx.NewBigInt64(int64(binary.LittleEndian.Uint64(data[offset : offset+8])))
}

func readBigUInt64BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 8)
	if !ok {
		return failure
	}
	return ctx.NewBigUint64(binary.BigEndian.Uint64(data[offset : offset+8]))
}

func readBigUInt64LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 8)
	if !ok {
		return failure
	}
	return ctx.NewBigUint64(binary.LittleEndian.Uint64(data[offset : offset+8]))
}

func readDoubleBE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 8)
	if !ok {
		return failure
	}
	return ctx.NewFloat64(math.Float64frombits(binary.BigEndian.Uint64(data[offset : offset+8])))
}

func readDoubleLE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 8)
	if !ok {
		return failure
	}
	return ctx.NewFloat64(math.Float64frombits(binary.LittleEndian.Uint64(data[offset : offset+8])))
}

func readFloatBE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 4)
	if !ok {
		return failure
	}
	return ctx.NewFloat64(float64(math.Float32frombits(binary.BigEndian.Uint32(data[offset : offset+4]))))
}

func readFloatLE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 4)
	if !ok {
		return failure
	}
	return ctx.NewFloat64(float64(math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))))
}

func readInt8(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 1)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(int8(data[offset])))
}

func readInt16BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 2)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(int16(binary.BigEndian.Uint16(data[offset : offset+2]))))
}

func readInt16LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 2)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(int16(binary.LittleEndian.Uint16(data[offset : offset+2]))))
}

func readInt32BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 4)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(int32(binary.BigEndian.Uint32(data[offset : offset+4]))))
}

func readInt32LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 4)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(int32(binary.LittleEndian.Uint32(data[offset : offset+4]))))
}

func readIntBE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, length, ok, failure := variableReadWindow(ctx, this, args)
	if !ok {
		return failure
	}
	var value int64
	for i := int64(0); i < length; i++ {
		value = (value << 8) | int64(data[offset+i])
	}
	return ctx.NewInt64(signExtend(value, length))
}

func readIntLE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, length, ok, failure := variableReadWindow(ctx, this, args)
	if !ok {
		return failure
	}
	var value int64
	for i := length - 1; i >= 0; i-- {
		value = (value << 8) | int64(data[offset+i])
	}
	return ctx.NewInt64(signExtend(value, length))
}

func readUInt8(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 1)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(data[offset]))
}

func readUInt16BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 2)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(binary.BigEndian.Uint16(data[offset : offset+2])))
}

func readUInt16LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 2)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(binary.LittleEndian.Uint16(data[offset : offset+2])))
}

func readUInt32BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 4)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(binary.BigEndian.Uint32(data[offset : offset+4])))
}

func readUInt32LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, ok, failure := readWindow(ctx, this, args, 4)
	if !ok {
		return failure
	}
	return ctx.NewInt64(int64(binary.LittleEndian.Uint32(data[offset : offset+4])))
}

func readUIntBE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, length, ok, failure := variableReadWindow(ctx, this, args)
	if !ok {
		return failure
	}
	var value uint64
	for i := int64(0); i < length; i++ {
		value = (value << 8) | uint64(data[offset+i])
	}
	return ctx.NewInt64(int64(value))
}

func readUIntLE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, offset, length, ok, failure := variableReadWindow(ctx, this, args)
	if !ok {
		return failure
	}
	var value uint64
	for i := length - 1; i >= 0; i-- {
		value = (value << 8) | uint64(data[offset+i])
	}
	return ctx.NewInt64(int64(value))
}

func writeBigInt64BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, offset, ok, failure := bigIntWriteArgs(ctx, this, args)
	if !ok {
		return failure
	}
	if !value.IsInt64() {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, uint64(value.Int64()))
	return writeWindow(ctx, this, offset, data)
}

func writeBigInt64LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, offset, ok, failure := bigIntWriteArgs(ctx, this, args)
	if !ok {
		return failure
	}
	if !value.IsInt64() {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, uint64(value.Int64()))
	return writeWindow(ctx, this, offset, data)
}

func writeBigUInt64BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, offset, ok, failure := bigIntWriteArgs(ctx, this, args)
	if !ok {
		return failure
	}
	if value.Sign() < 0 || !value.IsUint64() {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, value.Uint64())
	return writeWindow(ctx, this, offset, data)
}

func writeBigUInt64LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, offset, ok, failure := bigIntWriteArgs(ctx, this, args)
	if !ok {
		return failure
	}
	if value.Sign() < 0 || !value.IsUint64() {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, value.Uint64())
	return writeWindow(ctx, this, offset, data)
}

func writeDoubleBE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, ok, failure := floatWriteValue(ctx, args)
	if !ok {
		return failure
	}
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, math.Float64bits(value))
	return writeWindow(ctx, this, integerArgument(args, 1, 0), data)
}

func writeDoubleLE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, ok, failure := floatWriteValue(ctx, args)
	if !ok {
		return failure
	}
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, math.Float64bits(value))
	return writeWindow(ctx, this, integerArgument(args, 1, 0), data)
}

func writeFloatBE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, ok, failure := floatWriteValue(ctx, args)
	if !ok {
		return failure
	}
	if value < -math.MaxFloat32 || value > math.MaxFloat32 {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, math.Float32bits(float32(value)))
	return writeWindow(ctx, this, integerArgument(args, 1, 0), data)
}

func writeFloatLE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, ok, failure := floatWriteValue(ctx, args)
	if !ok {
		return failure
	}
	if value < -math.MaxFloat32 || value > math.MaxFloat32 {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, math.Float32bits(float32(value)))
	return writeWindow(ctx, this, integerArgument(args, 1, 0), data)
}

func writeInt8(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, ok, failure := integerWriteValue(ctx, args)
	if !ok {
		return failure
	}
	if value < math.MinInt8 || value > math.MaxInt8 {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	return writeWindow(ctx, this, integerArgument(args, 1, 0), []byte{byte(int8(value))})
}

func writeInt16BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return writeSigned16(ctx, this, args, true)
}

func writeInt16LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return writeSigned16(ctx, this, args, false)
}

func writeSigned16(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value, bigEndian bool) *quickjs.Value {
	value, ok, failure := integerWriteValue(ctx, args)
	if !ok {
		return failure
	}
	if value < math.MinInt16 || value > math.MaxInt16 {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 2)
	if bigEndian {
		binary.BigEndian.PutUint16(data, uint16(value))
	} else {
		binary.LittleEndian.PutUint16(data, uint16(value))
	}
	return writeWindow(ctx, this, integerArgument(args, 1, 0), data)
}

func writeInt32BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return writeSigned32(ctx, this, args, true)
}

func writeInt32LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return writeSigned32(ctx, this, args, false)
}

func writeSigned32(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value, bigEndian bool) *quickjs.Value {
	value, ok, failure := integerWriteValue(ctx, args)
	if !ok {
		return failure
	}
	if value < math.MinInt32 || value > math.MaxInt32 {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 4)
	if bigEndian {
		binary.BigEndian.PutUint32(data, uint32(value))
	} else {
		binary.LittleEndian.PutUint32(data, uint32(value))
	}
	return writeWindow(ctx, this, integerArgument(args, 1, 0), data)
}

func writeIntBE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, offset, length, ok, failure := variableWriteArgs(ctx, this, args)
	if !ok {
		return failure
	}
	min := -(int64(1) << (length*8 - 1))
	max := (int64(1) << (length*8 - 1)) - 1
	if value < min || value > max {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, length)
	for i := int64(0); i < length; i++ {
		data[i] = byte(value >> uint(8*(length-1-i)))
	}
	return writeWindow(ctx, this, offset, data)
}

func writeIntLE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, offset, length, ok, failure := variableWriteArgs(ctx, this, args)
	if !ok {
		return failure
	}
	min := -(int64(1) << (length*8 - 1))
	max := (int64(1) << (length*8 - 1)) - 1
	if value < min || value > max {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, length)
	for i := int64(0); i < length; i++ {
		data[i] = byte(value >> uint(8*i))
	}
	return writeWindow(ctx, this, offset, data)
}

func writeUInt8(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, ok, failure := integerWriteValue(ctx, args)
	if !ok {
		return failure
	}
	if value < 0 || value > math.MaxUint8 {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	return writeWindow(ctx, this, integerArgument(args, 1, 0), []byte{byte(value)})
}

func writeUInt16BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return writeUnsigned16(ctx, this, args, true)
}

func writeUInt16LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return writeUnsigned16(ctx, this, args, false)
}

func writeUnsigned16(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value, bigEndian bool) *quickjs.Value {
	value, ok, failure := integerWriteValue(ctx, args)
	if !ok {
		return failure
	}
	if value < 0 || value > math.MaxUint16 {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 2)
	if bigEndian {
		binary.BigEndian.PutUint16(data, uint16(value))
	} else {
		binary.LittleEndian.PutUint16(data, uint16(value))
	}
	return writeWindow(ctx, this, integerArgument(args, 1, 0), data)
}

func writeUInt32BE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return writeUnsigned32(ctx, this, args, true)
}

func writeUInt32LE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return writeUnsigned32(ctx, this, args, false)
}

func writeUnsigned32(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value, bigEndian bool) *quickjs.Value {
	value, ok, failure := integerWriteValue(ctx, args)
	if !ok {
		return failure
	}
	if value < 0 || uint64(value) > math.MaxUint32 {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, 4)
	if bigEndian {
		binary.BigEndian.PutUint32(data, uint32(value))
	} else {
		binary.LittleEndian.PutUint32(data, uint32(value))
	}
	return writeWindow(ctx, this, integerArgument(args, 1, 0), data)
}

func writeUIntBE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, offset, length, ok, failure := variableWriteArgs(ctx, this, args)
	if !ok {
		return failure
	}
	max := (int64(1) << (length * 8)) - 1
	if value < 0 || value > max {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, length)
	for i := int64(0); i < length; i++ {
		data[i] = byte(value >> uint(8*(length-1-i)))
	}
	return writeWindow(ctx, this, offset, data)
}

func writeUIntLE(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	value, offset, length, ok, failure := variableWriteArgs(ctx, this, args)
	if !ok {
		return failure
	}
	max := (int64(1) << (length * 8)) - 1
	if value < 0 || value > max {
		return throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	data := make([]byte, length)
	for i := int64(0); i < length; i++ {
		data[i] = byte(value >> uint(8*i))
	}
	return writeWindow(ctx, this, offset, data)
}

func readWindow(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value, width int64) ([]byte, int64, bool, *quickjs.Value) {
	data, ok := bytesFromValue(this)
	if !ok {
		return nil, 0, false, throwBufferTypeError(ctx, "Method called on incompatible receiver")
	}
	offset := integerArgument(args, 0, 0)
	if offset < 0 || offset > int64(len(data))-width {
		return nil, 0, false, throwBufferRangeError(ctx, "The \"offset\" argument is out of range")
	}
	return data, offset, true, nil
}

func variableReadWindow(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) ([]byte, int64, int64, bool, *quickjs.Value) {
	data, ok := bytesFromValue(this)
	if !ok {
		return nil, 0, 0, false, throwBufferTypeError(ctx, "Method called on incompatible receiver")
	}
	if len(args) < 2 {
		return nil, 0, 0, false, throwBufferTypeError(ctx, "The \"offset\" and \"byteLength\" arguments are required")
	}
	offset := args[0].ToInt64()
	length := args[1].ToInt64()
	if length < 1 || length > 6 {
		return nil, 0, 0, false, throwBufferRangeError(ctx, "The \"byteLength\" argument is out of range")
	}
	if offset < 0 || offset > int64(len(data))-length {
		return nil, 0, 0, false, throwBufferRangeError(ctx, "The \"offset\" argument is out of range")
	}
	return data, offset, length, true, nil
}

func writeWindow(ctx *quickjs.Context, this *quickjs.Value, offset int64, data []byte) *quickjs.Value {
	if this == nil || !this.IsUint8Array() && !this.IsUint8ClampedArray() {
		return throwBufferTypeError(ctx, "Method called on incompatible receiver")
	}
	if offset < 0 || offset > int64(lenBytes(this))-int64(len(data)) {
		return throwBufferRangeError(ctx, "The \"offset\" argument is out of range")
	}
	if !writeBytes(ctx, this, offset, data) {
		return throwBufferTypeError(ctx, "failed to write Buffer")
	}
	return ctx.NewInt64(offset + int64(len(data)))
}

func variableWriteArgs(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) (int64, int64, int64, bool, *quickjs.Value) {
	if len(args) < 3 {
		return 0, 0, 0, false, throwBufferTypeError(ctx, "The \"value\", \"offset\", and \"byteLength\" arguments are required")
	}
	value, ok, failure := integerWriteValue(ctx, args)
	if !ok {
		return 0, 0, 0, false, failure
	}
	data, dataOK := bytesFromValue(this)
	if !dataOK {
		return 0, 0, 0, false, throwBufferTypeError(ctx, "Method called on incompatible receiver")
	}
	offset := args[1].ToInt64()
	length := args[2].ToInt64()
	if length < 1 || length > 6 {
		return 0, 0, 0, false, throwBufferRangeError(ctx, "The \"byteLength\" argument is out of range")
	}
	if offset < 0 || offset > int64(len(data))-length {
		return 0, 0, 0, false, throwBufferRangeError(ctx, "The \"offset\" argument is out of range")
	}
	return value, offset, length, true, nil
}

func bigIntWriteArgs(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) (*big.Int, int64, bool, *quickjs.Value) {
	if len(args) == 0 || args[0] == nil || !args[0].IsBigInt() {
		return nil, 0, false, throwBufferTypeError(ctx, "The \"value\" argument must be a bigint")
	}
	value := args[0].ToBigInt()
	if value == nil {
		return nil, 0, false, throwBufferTypeError(ctx, "The \"value\" argument must be a bigint")
	}
	data, ok := bytesFromValue(this)
	if !ok {
		return nil, 0, false, throwBufferTypeError(ctx, "Method called on incompatible receiver")
	}
	offset := integerArgument(args, 1, 0)
	if offset < 0 || offset > int64(len(data))-8 {
		return nil, 0, false, throwBufferRangeError(ctx, "The \"offset\" argument is out of range")
	}
	return value, offset, true, nil
}

func floatWriteValue(ctx *quickjs.Context, args []*quickjs.Value) (float64, bool, *quickjs.Value) {
	if len(args) == 0 || args[0] == nil || !args[0].IsNumber() {
		return 0, false, throwBufferTypeError(ctx, "The \"value\" argument must be a number")
	}
	value := args[0].ToFloat64()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false, throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	return value, true, nil
}

func integerWriteValue(ctx *quickjs.Context, args []*quickjs.Value) (int64, bool, *quickjs.Value) {
	if len(args) == 0 || args[0] == nil || !args[0].IsNumber() {
		return 0, false, throwBufferTypeError(ctx, "The \"value\" argument must be a number")
	}
	value := args[0].ToFloat64()
	if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) || value < math.MinInt64 || value > math.MaxInt64 {
		return 0, false, throwBufferRangeError(ctx, "The \"value\" argument is out of range")
	}
	return int64(value), true, nil
}

func signExtend(value, byteLength int64) int64 {
	return (value << (64 - 8*byteLength)) >> (64 - 8*byteLength)
}
