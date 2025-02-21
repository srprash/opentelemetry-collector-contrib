// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fileconsumer // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/fileconsumer"

import (
	"bufio"
	"os"

	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator/helper"
)

type readerFactory struct {
	*zap.SugaredLogger
	readerConfig   *readerConfig
	fromBeginning  bool
	splitterConfig helper.SplitterConfig
	encodingConfig helper.EncodingConfig
}

func (f *readerFactory) newReader(file *os.File, fp *Fingerprint) (*Reader, error) {
	return f.newReaderBuilder().
		withFile(file).
		withFingerprint(fp).
		build()
}

// copy creates a deep copy of a Reader
func (f *readerFactory) copy(old *Reader, newFile *os.File) (*Reader, error) {
	return f.newReaderBuilder().
		withFile(newFile).
		withFingerprint(old.Fingerprint.Copy()).
		withOffset(old.Offset).
		withSplitterFunc(old.splitFunc).
		build()
}

func (f *readerFactory) unsafeReader() (*Reader, error) {
	return f.newReaderBuilder().build()
}

func (f *readerFactory) newFingerprint(file *os.File) (*Fingerprint, error) {
	return NewFingerprint(file, f.readerConfig.fingerprintSize)
}

type readerBuilder struct {
	*readerFactory
	file      *os.File
	fp        *Fingerprint
	offset    int64
	splitFunc bufio.SplitFunc
}

func (f *readerFactory) newReaderBuilder() *readerBuilder {
	return &readerBuilder{readerFactory: f}
}

func (b *readerBuilder) withSplitterFunc(s bufio.SplitFunc) *readerBuilder {
	b.splitFunc = s
	return b
}

func (b *readerBuilder) withFile(f *os.File) *readerBuilder {
	b.file = f
	return b
}

func (b *readerBuilder) withFingerprint(fp *Fingerprint) *readerBuilder {
	b.fp = fp
	return b
}

func (b *readerBuilder) withOffset(offset int64) *readerBuilder {
	b.offset = offset
	return b
}

func (b *readerBuilder) build() (r *Reader, err error) {
	r = &Reader{
		readerConfig: b.readerConfig,
		Offset:       b.offset,
	}

	var splitter *helper.Splitter
	if b.splitFunc != nil {
		r.splitFunc = b.splitFunc
	} else {
		splitter, err = b.splitterConfig.Build(false, b.readerConfig.maxLogSize)
		r.splitFunc = splitter.SplitFunc
		if err != nil {
			return
		}
	}

	enc, err := b.encodingConfig.Build()
	if err != nil {
		return
	}
	r.encoding = enc

	if b.file != nil {
		r.file = b.file
		r.SugaredLogger = b.SugaredLogger.With("path", b.file.Name())
		r.fileAttributes, err = resolveFileAttributes(b.file.Name())
		if err != nil {
			b.Errorf("resolve attributes: %w", err)
		}

		// unsafeReader has the file set to nil, so don't try emending its offset.
		if !b.fromBeginning {
			if err := r.offsetToEnd(); err != nil {
				return nil, err
			}
		}
	} else {
		r.SugaredLogger = b.SugaredLogger.With("path", "uninitialized")
	}

	if b.fp != nil {
		r.Fingerprint = b.fp
	} else if b.file != nil {
		fp, err := b.readerFactory.newFingerprint(r.file)
		if err != nil {
			return nil, err
		}
		r.Fingerprint = fp
	}

	return r, nil
}
