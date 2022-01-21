package viperutil

import (
	"reflect"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

func SetDefault(in interface{}) {
	cfg, err := decode(in)
	if err != nil {
		panic(err)
	}
	mcfg, ok := cfg.(map[string]interface{})
	if !ok {
		panic("only support struct config")
	}
	for k, v := range mcfg {
		viper.SetDefault(k, v)
	}
}

func VSetDefault(vp *viper.Viper, in interface{}) {
	cfg, err := decode(in)
	if err != nil {
		panic(err)
	}
	mcfg, ok := cfg.(map[string]interface{})
	if !ok {
		panic("only support struct config")
	}
	for k, v := range mcfg {
		vp.SetDefault(k, v)
	}
}

func decode(in interface{}) (_ interface{}, err error) {
	if isStruct(in) {
		out := map[string]interface{}{}
		if err = mapstructure.Decode(in, &out); err != nil {
			return nil, err
		}
		for k, v := range out {
			if isDecodable(v) {
				if out[k], err = decode(v); err != nil {
					return nil, err
				}
			}
		}
		return out, nil
	} else if isArrayOfStruct(in) {
		val := reflect.ValueOf(in)
		out := make([]interface{}, val.Len())
		for i := 0; i < val.Len(); i++ {
			if out[i], err = decode(val.Index(i).Interface()); err != nil {
				return nil, err
			}
		}
		return out, nil
	} else if m, ok := in.(map[string]interface{}); ok {
		for k, v := range m {
			if m[k], err = decode(v); err != nil {
				return nil, err
			}
		}
		return m, nil
	}
	return in, nil
}

func isDecodable(v interface{}) bool {
	typ := reflect.TypeOf(v)
	if typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Kind() == reflect.Struct || typ.Kind() == reflect.Map
}

func isStruct(v interface{}) bool {
	typ := reflect.TypeOf(v)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Kind() == reflect.Struct
}

func isArrayOfStruct(v interface{}) bool {
	typ := reflect.TypeOf(v)
	if typ.Kind() == reflect.Slice {
		typ = typ.Elem()
		if typ.Kind() == reflect.Ptr {
			typ = typ.Elem()
		}
		return typ.Kind() == reflect.Struct
	}
	return false
}
