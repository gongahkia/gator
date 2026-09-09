package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/gongahkia/gator/internal/eval"
)

func workRubricCommand(action, path string, args []string, out io.Writer) error {
	if action == "calibrate-rubric" {
		if len(args) != 1 {
			return errors.New("usage: gator eval work calibrate-rubric JUDGE.json HUMAN.json")
		}
		var judge, human eval.RubricReport
		if err := readWorkJSON(path, &judge); err != nil {
			return err
		}
		if err := readWorkJSON(args[0], &human); err != nil {
			return err
		}
		result, err := eval.CalibrateRubric(judge, human)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(result)
	}
	flags := flag.NewFlagSet("judge-rubric", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	rubricPath := flags.String("rubric", "", "independent rubric v1 JSON")
	output := flags.String("output", "", "new rubric result JSON file")
	provider := flags.String("provider", "", "judge provider")
	model := flags.String("model", "", "judge model")
	requests := flags.Int("max-model-requests", 0, "aggregate judge request budget")
	seconds := flags.Int("timeout-seconds", 0, "judge wall budget")
	live := flags.Bool("live", false, "send retained textual artifacts/evidence to the independent judge")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*live || *requests < 1 || *seconds < 1 || *provider == "" || *model == "" || *output == "" || *rubricPath == "" {
		return errors.New("judge-rubric requires --live, --rubric, --output, --provider, --model, --max-model-requests and --timeout-seconds")
	}
	report, err := eval.LoadWorkExperiment(path)
	if err != nil {
		return err
	}
	var rubric eval.Rubric
	if err := readWorkJSON(*rubricPath, &rubric); err != nil {
		return err
	}
	if err := rubric.Validate(); err != nil {
		return err
	}
	backend, err := nativeWorkModel(*provider, *model, "")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*seconds)*time.Second)
	defer cancel()
	result, err := eval.JudgeWorkRubric(ctx, backend, report, rubric, *provider+"/"+*model, *requests)
	if err != nil {
		return err
	}
	if err := eval.SaveRubric(*output, result); err != nil {
		return err
	}
	if err := json.NewEncoder(out).Encode(result); err != nil {
		return err
	}
	for _, trial := range result.Trials {
		if trial.Error != "" {
			return errors.New("rubric grading has failures; retained in output")
		}
	}
	return nil
}
