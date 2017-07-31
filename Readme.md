Aggr (wip)
==========

Aggregate outputs from a list of commands.

## Install

```sh
go get -u github.com/yazgazan/aggr
```

## Usage

```
Usage of aggr:
  -cmd string
    	command template (default "echo {{.Arg}}")
  -input string
    	input file (csv) (default "/dev/stdin")
```

## Example

``` sh
kubectl get pods -o json | jq -r '.items[].metadata.name' | aggr -cmd="kubectl logs -f {{.Arg}}"
```

## TODO

- [ ] Automatically restart process
