// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"sort"
	"unsafe"

	"github.com/kisielk/gotool"
	"golang.org/x/tools/go/loader"
	"go/types"
)

var stdSizes = types.StdSizes{
	WordSize: int64(unsafe.Sizeof(int(0))),
	MaxAlign: 8,
}

func main() {
	flag.Parse()
	exitStatus := 0

	importPaths := gotool.ImportPaths(flag.Args())
	if len(importPaths) == 0 {
		importPaths = []string{"."}
	}

	ctx := build.Default
	loadcfg := loader.Config{
		Build: &ctx,
	}
	rest, err := loadcfg.FromArgs(importPaths, false)
	if err != nil {
		log.Fatalf("could not parse arguments: %s", err)
	}
	if len(rest) > 0 {
		log.Fatalf("unhandled extra arguments: %v", rest)
	}

	program, err := loadcfg.Load()
	if err != nil {
		log.Fatalf("could not type check: %s", err)
	}

	var lines []string

	for _, pkgInfo := range program.InitialPackages() {
		for _, obj := range pkgInfo.Defs {
			if obj == nil {
				continue
			}

			if _, ok := obj.(*types.TypeName); !ok {
				continue
			}

			typ, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}

			strukt, ok := typ.Underlying().(*types.Struct)
			if !ok {
				continue
			}

			structAlign := int(stdSizes.Alignof(strukt))
			structSize := int(stdSizes.Sizeof(strukt))
			if structSize%structAlign != 0 {
				structSize += structAlign - structSize%structAlign
			}

			minSize := 0
			for i := 0; i < strukt.NumFields(); i++ {
				field := strukt.Field(i)
				fieldType := field.Type()
				typeSize := int(stdSizes.Sizeof(fieldType))
				minSize += typeSize
			}
			if minSize%structAlign != 0 {
				minSize += structAlign - minSize%structAlign
			}

			if minSize != structSize {
				pos := program.Fset.Position(obj.Pos())
				lines = append(lines, fmt.Sprintf(
					"%s: %s:%d:%d: struct %s could have size %d (currently %d)",
					obj.Pkg().Path(),
					pos.Filename,
					pos.Line,
					pos.Column,
					obj.Name(),
					minSize,
					structSize,
				))
				exitStatus = 1
			}
		}
	}

	sort.Strings(lines)
	for _, line := range lines {
		fmt.Println(line)
	}

	os.Exit(exitStatus)
}
