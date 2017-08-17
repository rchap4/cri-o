package main

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/cmd/kpod/formats"
	libkpodimage "github.com/kubernetes-incubator/cri-o/libkpod/image"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var (
	imagesFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "display only image IDs",
		},
		cli.BoolFlag{
			Name:  "noheading, n",
			Usage: "do not print column headings",
		},
		cli.BoolFlag{
			Name:  "no-trunc, notruncate",
			Usage: "do not truncate output",
		},
		cli.BoolFlag{
			Name:  "digests",
			Usage: "show digests",
		},
		cli.StringFlag{
			Name:  "format",
			Usage: "Change the output format.",
		},
		cli.StringFlag{
			Name:  "filter, f",
			Usage: "filter output based on conditions provided (default [])",
		},
	}

	imagesDescription = "lists locally stored images."
	imagesCommand     = cli.Command{
		Name:        "images",
		Usage:       "list images in local storage",
		Description: imagesDescription,
		Flags:       imagesFlags,
		Action:      imagesCmd,
		ArgsUsage:   "",
	}
)

func imagesCmd(c *cli.Context) error {
	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "Could not get config")
	}
	store, err := getStore(config)
	if err != nil {
		return err
	}

	quiet := false
	if c.IsSet("quiet") {
		quiet = c.Bool("quiet")
	}
	noheading := false
	if c.IsSet("noheading") {
		noheading = c.Bool("noheading")
	}
	truncate := true
	if c.IsSet("no-trunc") {
		truncate = !c.Bool("no-trunc")
	}
	digests := false
	if c.IsSet("digests") {
		digests = c.Bool("digests")
	}
	outputFormat := genImagesFormat(quiet, truncate, digests)
	if c.IsSet("format") {
		outputFormat = c.String("format")
	}

	name := ""
	if len(c.Args()) == 1 {
		name = c.Args().Get(0)
	} else if len(c.Args()) > 1 {
		return errors.New("'kpod images' requires at most 1 argument")
	}

	var params *libkpodimage.FilterParams
	if c.IsSet("filter") {
		params, err = libkpodimage.ParseFilter(store, c.String("filter"))
		if err != nil {
			return errors.Wrapf(err, "error parsing filter")
		}
	} else {
		params = nil
	}

	imageList, err := libkpodimage.GetImagesMatchingFilter(store, params, name)
	if err != nil {
		return errors.Wrapf(err, "could not get list of images matching filter")
	}

	return outputImages(store, imageList, truncate, digests, quiet, outputFormat, noheading)
}

func genImagesFormat(quiet, truncate, digests bool) (format string) {
	if quiet {
		return "{{.ID}}"
	}
	if truncate {
		format = "table {{ .ID | printf \"%-20.12s\" }} "
	} else {
		format = "table {{ .ID | printf \"%-64s\" }} "
	}
	format += "{{ .Name | printf \"%-56s\" }} "

	if digests {
		format += "{{ .Digest | printf \"%-71s \"}} "
	}

	format += "{{ .CreatedAt | printf \"%-22s\" }} {{.Size}}"
	return
}

func outputImages(store storage.Store, images []storage.Image, truncate, digests, quiet bool, outputFormat string, noheading bool) error {
	imageOutput := []imageOutputParams{}

	lastID := ""
	for _, img := range images {
		if quiet && lastID == img.ID {
			continue // quiet should not show the same ID multiple times
		}
		createdTime := img.Created

		name := ""
		if len(img.Names) > 0 {
			name = img.Names[0]
		}

		info, imageDigest, size, _ := libkpodimage.InfoAndDigestAndSize(store, img)
		if info != nil {
			createdTime = info.Created
		}

		params := imageOutputParams{
			ID:        img.ID,
			Name:      name,
			Digest:    imageDigest,
			CreatedAt: createdTime.Format("Jan 2, 2006 15:04"),
			Size:      libkpodimage.FormattedSize(size),
		}
		imageOutput = append(imageOutput, params)
	}

	var out formats.Writer

	switch outputFormat {
	case "json":
		out = formats.JSONstruct{Output: toGeneric(imageOutput)}
	default:
		out = formats.StdoutTemplate{Output: toGeneric(imageOutput), Template: outputFormat, Fields: imageOutput[0].headerMap()}
	}

	formats.Writer(out).Out()

	return nil
}

type imageOutputParams struct {
	ID        string        `json:"id"`
	Name      string        `json:"names"`
	Digest    digest.Digest `json:"digest"`
	CreatedAt string        `json:"created"`
	Size      string        `json:"size"`
}

func toGeneric(params []imageOutputParams) []interface{} {
	genericParams := make([]interface{}, len(params))
	for i, v := range params {
		genericParams[i] = interface{}(v)
	}
	return genericParams
}

func (i *imageOutputParams) headerMap() map[string]string {
	v := reflect.Indirect(reflect.ValueOf(i))
	values := make(map[string]string)

	for i := 0; i < v.NumField(); i++ {
		key := v.Type().Field(i).Name
		value := key
		if value == "ID" || value == "Name" {
			value = "Image" + value
		}
		values[key] = fmt.Sprintf("%s        ", strings.ToUpper(splitCamelCase(value)))
	}
	return values
}