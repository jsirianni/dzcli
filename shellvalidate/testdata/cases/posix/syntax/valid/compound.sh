if test -n "$value"; then
  while read -r line; do printf '%s\n' "$line"; done
else
  :
fi
