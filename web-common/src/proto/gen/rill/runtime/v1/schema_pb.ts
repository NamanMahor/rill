// @generated by protoc-gen-es v1.1.1 with parameter "target=ts"
// @generated from file rill/runtime/v1/schema.proto (package rill.runtime.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import type { BinaryReadOptions, FieldList, JsonReadOptions, JsonValue, PartialMessage, PlainMessage } from "@bufbuild/protobuf";
import { Message, proto3 } from "@bufbuild/protobuf";

/**
 * Type represents a data type in a schema
 *
 * @generated from message rill.runtime.v1.Type
 */
export class Type extends Message<Type> {
  /**
   * Code designates the type
   *
   * @generated from field: rill.runtime.v1.Type.Code code = 1;
   */
  code = Type_Code.UNSPECIFIED;

  /**
   * Nullable indicates whether null values are possible
   *
   * @generated from field: bool nullable = 2;
   */
  nullable = false;

  /**
   * If code is CODE_ARRAY, array_element_type specifies the type of the array elements
   *
   * @generated from field: rill.runtime.v1.Type array_element_type = 3;
   */
  arrayElementType?: Type;

  /**
   * If code is CODE_STRUCT, struct_type specifies the type of the struct's fields
   *
   * @generated from field: rill.runtime.v1.StructType struct_type = 4;
   */
  structType?: StructType;

  /**
   * If code is CODE_MAP, map_type specifies the map's key and value types
   *
   * @generated from field: rill.runtime.v1.MapType map_type = 5;
   */
  mapType?: MapType;

  constructor(data?: PartialMessage<Type>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "rill.runtime.v1.Type";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "code", kind: "enum", T: proto3.getEnumType(Type_Code) },
    { no: 2, name: "nullable", kind: "scalar", T: 8 /* ScalarType.BOOL */ },
    { no: 3, name: "array_element_type", kind: "message", T: Type },
    { no: 4, name: "struct_type", kind: "message", T: StructType },
    { no: 5, name: "map_type", kind: "message", T: MapType },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): Type {
    return new Type().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): Type {
    return new Type().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): Type {
    return new Type().fromJsonString(jsonString, options);
  }

  static equals(a: Type | PlainMessage<Type> | undefined, b: Type | PlainMessage<Type> | undefined): boolean {
    return proto3.util.equals(Type, a, b);
  }
}

/**
 * Code enumerates all the types that can be represented in a schema
 *
 * @generated from enum rill.runtime.v1.Type.Code
 */
export enum Type_Code {
  /**
   * @generated from enum value: CODE_UNSPECIFIED = 0;
   */
  UNSPECIFIED = 0,

  /**
   * @generated from enum value: CODE_BOOL = 1;
   */
  BOOL = 1,

  /**
   * @generated from enum value: CODE_INT8 = 2;
   */
  INT8 = 2,

  /**
   * @generated from enum value: CODE_INT16 = 3;
   */
  INT16 = 3,

  /**
   * @generated from enum value: CODE_INT32 = 4;
   */
  INT32 = 4,

  /**
   * @generated from enum value: CODE_INT64 = 5;
   */
  INT64 = 5,

  /**
   * @generated from enum value: CODE_INT128 = 6;
   */
  INT128 = 6,

  /**
   * @generated from enum value: CODE_UINT8 = 7;
   */
  UINT8 = 7,

  /**
   * @generated from enum value: CODE_UINT16 = 8;
   */
  UINT16 = 8,

  /**
   * @generated from enum value: CODE_UINT32 = 9;
   */
  UINT32 = 9,

  /**
   * @generated from enum value: CODE_UINT64 = 10;
   */
  UINT64 = 10,

  /**
   * @generated from enum value: CODE_UINT128 = 11;
   */
  UINT128 = 11,

  /**
   * @generated from enum value: CODE_FLOAT32 = 12;
   */
  FLOAT32 = 12,

  /**
   * @generated from enum value: CODE_FLOAT64 = 13;
   */
  FLOAT64 = 13,

  /**
   * @generated from enum value: CODE_TIMESTAMP = 14;
   */
  TIMESTAMP = 14,

  /**
   * @generated from enum value: CODE_DATE = 15;
   */
  DATE = 15,

  /**
   * @generated from enum value: CODE_TIME = 16;
   */
  TIME = 16,

  /**
   * @generated from enum value: CODE_STRING = 17;
   */
  STRING = 17,

  /**
   * @generated from enum value: CODE_BYTES = 18;
   */
  BYTES = 18,

  /**
   * @generated from enum value: CODE_ARRAY = 19;
   */
  ARRAY = 19,

  /**
   * @generated from enum value: CODE_STRUCT = 20;
   */
  STRUCT = 20,

  /**
   * @generated from enum value: CODE_MAP = 21;
   */
  MAP = 21,

  /**
   * @generated from enum value: CODE_DECIMAL = 22;
   */
  DECIMAL = 22,

  /**
   * @generated from enum value: CODE_JSON = 23;
   */
  JSON = 23,

  /**
   * @generated from enum value: CODE_UUID = 24;
   */
  UUID = 24,
}
// Retrieve enum metadata with: proto3.getEnumType(Type_Code)
proto3.util.setEnumType(Type_Code, "rill.runtime.v1.Type.Code", [
  { no: 0, name: "CODE_UNSPECIFIED" },
  { no: 1, name: "CODE_BOOL" },
  { no: 2, name: "CODE_INT8" },
  { no: 3, name: "CODE_INT16" },
  { no: 4, name: "CODE_INT32" },
  { no: 5, name: "CODE_INT64" },
  { no: 6, name: "CODE_INT128" },
  { no: 7, name: "CODE_UINT8" },
  { no: 8, name: "CODE_UINT16" },
  { no: 9, name: "CODE_UINT32" },
  { no: 10, name: "CODE_UINT64" },
  { no: 11, name: "CODE_UINT128" },
  { no: 12, name: "CODE_FLOAT32" },
  { no: 13, name: "CODE_FLOAT64" },
  { no: 14, name: "CODE_TIMESTAMP" },
  { no: 15, name: "CODE_DATE" },
  { no: 16, name: "CODE_TIME" },
  { no: 17, name: "CODE_STRING" },
  { no: 18, name: "CODE_BYTES" },
  { no: 19, name: "CODE_ARRAY" },
  { no: 20, name: "CODE_STRUCT" },
  { no: 21, name: "CODE_MAP" },
  { no: 22, name: "CODE_DECIMAL" },
  { no: 23, name: "CODE_JSON" },
  { no: 24, name: "CODE_UUID" },
]);

/**
 * StructType is a type composed of ordered, named and typed sub-fields
 *
 * @generated from message rill.runtime.v1.StructType
 */
export class StructType extends Message<StructType> {
  /**
   * @generated from field: repeated rill.runtime.v1.StructType.Field fields = 1;
   */
  fields: StructType_Field[] = [];

  constructor(data?: PartialMessage<StructType>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "rill.runtime.v1.StructType";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "fields", kind: "message", T: StructType_Field, repeated: true },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): StructType {
    return new StructType().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): StructType {
    return new StructType().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): StructType {
    return new StructType().fromJsonString(jsonString, options);
  }

  static equals(a: StructType | PlainMessage<StructType> | undefined, b: StructType | PlainMessage<StructType> | undefined): boolean {
    return proto3.util.equals(StructType, a, b);
  }
}

/**
 * @generated from message rill.runtime.v1.StructType.Field
 */
export class StructType_Field extends Message<StructType_Field> {
  /**
   * @generated from field: string name = 1;
   */
  name = "";

  /**
   * @generated from field: rill.runtime.v1.Type type = 2;
   */
  type?: Type;

  constructor(data?: PartialMessage<StructType_Field>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "rill.runtime.v1.StructType.Field";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "name", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "type", kind: "message", T: Type },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): StructType_Field {
    return new StructType_Field().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): StructType_Field {
    return new StructType_Field().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): StructType_Field {
    return new StructType_Field().fromJsonString(jsonString, options);
  }

  static equals(a: StructType_Field | PlainMessage<StructType_Field> | undefined, b: StructType_Field | PlainMessage<StructType_Field> | undefined): boolean {
    return proto3.util.equals(StructType_Field, a, b);
  }
}

/**
 * MapType is a complex type for mapping keys to values
 *
 * @generated from message rill.runtime.v1.MapType
 */
export class MapType extends Message<MapType> {
  /**
   * @generated from field: rill.runtime.v1.Type key_type = 1;
   */
  keyType?: Type;

  /**
   * @generated from field: rill.runtime.v1.Type value_type = 2;
   */
  valueType?: Type;

  constructor(data?: PartialMessage<MapType>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "rill.runtime.v1.MapType";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "key_type", kind: "message", T: Type },
    { no: 2, name: "value_type", kind: "message", T: Type },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): MapType {
    return new MapType().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): MapType {
    return new MapType().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): MapType {
    return new MapType().fromJsonString(jsonString, options);
  }

  static equals(a: MapType | PlainMessage<MapType> | undefined, b: MapType | PlainMessage<MapType> | undefined): boolean {
    return proto3.util.equals(MapType, a, b);
  }
}

