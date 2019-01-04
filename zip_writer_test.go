package gopc

import (
	"archive/zip"
	"bytes"
	"reflect"
	"testing"
)

func TestNewWriter(t *testing.T) {
	tests := []struct {
		name  string
		want  *Writer
		wantW string
	}{
		{"base", &Writer{Package: newPackage(), w: zip.NewWriter(&bytes.Buffer{})}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &bytes.Buffer{}
			if got := NewWriter(w); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewWriter() = %v, want %v", got, tt.want)
			}
			if gotW := w.String(); gotW != tt.wantW {
				t.Errorf("NewWriter() = %v, want %v", gotW, tt.wantW)
			}
		})
	}
}

func TestWriter_Flush(t *testing.T) {
	tests := []struct {
		name    string
		w       *Writer
		wantErr bool
	}{
		{"base", NewWriter(&bytes.Buffer{}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.w.Flush(); (err != nil) != tt.wantErr {
				t.Errorf("Writer.Flush() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWriter_Close(t *testing.T) {
	tests := []struct {
		name    string
		w       *Writer
		wantErr bool
	}{
		{"base", NewWriter(&bytes.Buffer{}), false},
		{"withLast", &Writer{Package: newPackage(), w: zip.NewWriter(&bytes.Buffer{}), last: &Part{relationer: relationer{}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.w.Close(); (err != nil) != tt.wantErr {
				t.Errorf("Writer.Close() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWriter_setCompressor(t *testing.T) {
	type args struct {
		fh          *zip.FileHeader
		compression CompressionOption
	}
	tests := []struct {
		name       string
		w          *Writer
		args       args
		wantFlag   uint16
		wantMethod uint16
	}{
		{"none", NewWriter(nil), args{&zip.FileHeader{}, CompressionNone}, 0x0, zip.Store},
		{"normal", NewWriter(nil), args{&zip.FileHeader{}, CompressionNormal}, 0x0, zip.Deflate},
		{"max", NewWriter(nil), args{&zip.FileHeader{}, CompressionMaximum}, 0x2, zip.Deflate},
		{"fast", NewWriter(nil), args{&zip.FileHeader{}, CompressionFast}, 0x4, zip.Deflate},
		{"sfast", NewWriter(nil), args{&zip.FileHeader{}, CompressionSuperFast}, 0x6, zip.Deflate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.w.setCompressor(tt.args.fh, tt.args.compression)
			if tt.args.fh.Method != tt.wantMethod {
				t.Error("Writer.setCompressor() should have set the method flag the deflate")
			}
		})
	}
}

func Test_compressionFunc(t *testing.T) {
	type args struct {
		comp int
	}
	tests := []struct {
		name string
		args args
	}{
		{"base", args{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressionFunc(tt.args.comp)(&bytes.Buffer{})
		})
	}
}
