#!/usr/bin/env bash
values=(one two)
if [[ ${values[0]} =~ ^o ]]; then
  for ((i=0; i<2; i++)); do printf '%s\n' "${values[i]}"; done
fi
