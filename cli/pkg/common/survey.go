package common

import (
	"fmt"

	"github.com/solo-io/supergloo/cli/pkg/cmd/options"
	"gopkg.in/AlecAivazis/survey.v1"
)

func ChooseNamespace(opts *options.Options, message string) (string, error) {

	question := &survey.Select{
		Message: message,
		Options: opts.Cache.Namespaces,
	}

	var choice string
	if err := survey.AskOne(question, &choice, survey.Required); err != nil {
		// this should not error
		fmt.Println("error with input")
		return "", err
	}

	return choice, nil
}
