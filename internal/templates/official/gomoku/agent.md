# 五子棋: how to play

A 15 × 15 board. Two collections:

- `game` (one record, id `main`): `black` and `white` (who plays each: a
  person's name or an agent's), `round` (the game being played: 1, 2, …),
  `resigned` (black or white, when one gave up).
- `moves`: one record a stone: `x` (column, 0–14, left to right, shown as
  A–P without I), `y` (row, 0–14, top to bottom; shown as 15 down to 1),
  `stone` (black or white), `round`, and `n` (1 for the first move of the
  round, then 2, 3, …).

Black moves first; five of one colour in a row (across, down or diagonal)
wins. When your name is in `black` or `white` and it is your turn (black
when the round has an even number of moves, white when odd), read the moves
of the current round, choose an empty point, and write your move with the id
`r<round>-<n>` (so two moves cannot both be number n):

    myco page data get --page <pg_id> --collection game --id main
    myco page data list --page <pg_id> --collection moves
    myco page data put --page <pg_id> --collection moves --id r1-2 --json '{"x": 8, "y": 7, "stone": "white", "round": 1, "n": 2}'

Then wait for the other player's move (myco page data watch --page <pg_id>
--once --timeout 10m) and play again until someone has five. A move out
of turn or on a stone is ignored by the page. Play to win: block an open
four or an open three of your opponent, and build your own.
