# Lottery Ticket Search — Design Proposal

The system has to solve **two problems**:

1. **Find matching tickets fast:** search 10 million tickets with a wildcard
   pattern like `****23`.
2. **Never give the same ticket to two people:** when many users search the
   same pattern at the same moment, each one must get different tickets.

**The key rule:** a search does not just *read* tickets. It **reserves**
the tickets it returns, in one database statement. Once a ticket is
reserved, no other search can see it.

```
AVAILABLE  ──search──▶  RESERVED (by one user, 5 min)  ──buy──▶  SOLD
    ▲                          │
    └──────── expired ─────────┘
```

---

## 1. Facts about the data

- A ticket number has 6 digits, so there are only **1,000,000 possible
  numbers** (`000000`–`999999`).
- With 10 million tickets, each number is printed **about 10 times**
  (different sets), like the Thai Government Lottery.
- A pattern fixes some digits and uses `*` for the rest. Only the **fixed
  digits** matter. `****23` simply means "digit 5 is 2 **and** digit 6 is 3".

---

## 2. Storage choice: PostgreSQL

| Need | Why PostgreSQL fits |
| --- | --- |
| Never give one ticket to two users | Row locks + `FOR UPDATE SKIP LOCKED` (built in) |
| Never sell one ticket twice | ACID transactions |
| 10M rows | Small for Postgres (≈1 GB; fits in RAM) |
| Operational simplicity | One database, well known, easy backups/replicas |

---

## 3. Problem 1: finding matching tickets fast

### 3.1 Split the number into 6 digit columns

Each digit is stored in its own column, so every digit position can have
its own index:

| id | number | d1 | d2 | d3 | d4 | d5 | d6 |
|---|---|---|---|---|---|---|---|
| 1 | 482723 | 4 | 8 | 2 | 7 | **2** | **3** |
| 2 | 150023 | 1 | 5 | 0 | 0 | **2** | **3** |
| 3 | 123456 | 1 | 2 | 3 | 4 | 5 | 6 |
| 4 | 990025 | 9 | 9 | 0 | 0 | 2 | 5 |

Each fixed digit in the pattern becomes one condition. Each `*` is
skipped:

| Pattern | Meaning | WHERE clause |
| --- | --- | --- |
| `****23` | ends with 23 | `d5 = '2' AND d6 = '3'` (tickets 1, 2) |
| `1****5` | starts with 1, ends with 5 | `d1 = '1' AND d6 = '5'` |
| `123***` | starts with 123 | `number BETWEEN '123000' AND '123999'` |
| `******` | anything | *(no digit condition)* |

A **prefix pattern** like `123***` matches one continuous range of numbers,
so the index on `number` finds it with a single range scan.

### 3.2 Combining indexes: BitmapAnd

When a pattern fixes several digits, PostgreSQL turns each digit index into
a bitmap of matching rows and ANDs the bitmaps together. It then reads only
the rows that match every condition. In simplified form:

```
ticket:       1  2  3  4
d5 = '2'      1  1  0  1
d6 = '3'      1  1  0  0
             ────────────  AND
result        1  1  0  0   → tickets 1 and 2 match ****23
```

The real bitmap records the table pages and row IDs (TIDs) that match, not
one bit per ticket. The AND happens in memory and is far cheaper than
reading the table. We only create the indexes; PostgreSQL decides on its own
when to use BitmapAnd:

```
Bitmap Heap Scan on tickets          ← 3) read only the matching rows
  -> BitmapAnd                       ← 2) AND the two bitmaps
       -> Bitmap Index Scan on ix_d5 ← 1) bitmap from the d5 index
       -> Bitmap Index Scan on ix_d6 ← 1) bitmap from the d6 index
```

**When the planner chooses differently:** a single digit matches about 10%
of tickets, so while many tickets still match, a query with `LIMIT 5` is
often cheaper as a plain scan that stops after 5 hits. BitmapAnd becomes
the faster plan when matches are rare. The partial indexes keep that case
fast as tickets sell out.

### 3.3 Table and indexes

```sql
CREATE TABLE tickets (
  id              BIGSERIAL PRIMARY KEY,
  draw_id         INT       NOT NULL,             -- draw date
  number          CHAR(6)   NOT NULL,             -- '482723' (keeps leading zeros)
  set_no          SMALLINT  NOT NULL,             -- which copy of the number
  status          TEXT      NOT NULL DEFAULT 'AVAILABLE',  -- AVAILABLE | RESERVED | SOLD
  reserved_by     BIGINT,
  reserved_until  TIMESTAMPTZ,
  sold_to         BIGINT,
  -- one column per digit, filled automatically by the database
  d1 CHAR(1) GENERATED ALWAYS AS (substr(number,1,1)) STORED,
  d2 CHAR(1) GENERATED ALWAYS AS (substr(number,2,1)) STORED,
  d3 CHAR(1) GENERATED ALWAYS AS (substr(number,3,1)) STORED,
  d4 CHAR(1) GENERATED ALWAYS AS (substr(number,4,1)) STORED,
  d5 CHAR(1) GENERATED ALWAYS AS (substr(number,5,1)) STORED,
  d6 CHAR(1) GENERATED ALWAYS AS (substr(number,6,1)) STORED,
  UNIQUE (draw_id, number, set_no)
);

-- Indexes only cover AVAILABLE tickets, so they shrink as tickets sell.
CREATE INDEX ix_number ON tickets (draw_id, number) WHERE status = 'AVAILABLE';
CREATE INDEX ix_d1 ON tickets (draw_id, d1) WHERE status = 'AVAILABLE';
-- ... same for d2, d3, d4, d5, d6
```

The application validates the pattern with `^[0-9*]{6}$` and builds the
query with **parameters only** (never string concatenation).

---

## 4. Problem 2: never giving the same ticket to two people

### 4.1 Why the obvious approach breaks

The obvious approach takes two steps: `SELECT` free tickets, then `UPDATE`
them to reserved. If Alice and Bob search at the same moment:

```
time   Alice                              Bob
0 ms   SELECT → tickets 1-5 are free
1 ms                                      SELECT → tickets 1-5 are free too
2 ms   UPDATE 1-5 → reserved for Alice
3 ms                                      UPDATE 1-5 → reserved for Bob   <- duplicate!
```

The other user slips into the gap between "look" and "reserve". This is a
**race condition**.

### 4.2 The fix: search and reserve in one statement

```sql
WITH picked AS (
  SELECT id
  FROM   tickets
  WHERE  draw_id = $1
    AND  status = 'AVAILABLE'
    AND  d5 = '2' AND d6 = '3'        -- built from the pattern
  LIMIT  5
  FOR UPDATE SKIP LOCKED              -- skip rows someone else is taking right now
)
UPDATE tickets t
SET    status = 'RESERVED',
       reserved_by = $2,
       reserved_until = now() + interval '5 minutes'
FROM   picked
WHERE  t.id = picked.id
RETURNING t.id, t.number, t.set_no;
```

- **`FOR UPDATE`** locks the rows this search picks, so no one else can
  take them.
- **`SKIP LOCKED`** makes other searches skip those rows instead of waiting
  for them. Concurrent users therefore never queue behind each other.

```
time   Alice                                   Bob
0 ms   pick + lock 1-5 → label RESERVED
1 ms                                           1-5 are locked → skip → pick 6-10
       gets 1-5 (OK)                              gets 6-10 (OK)
```

Afterwards tickets 1-5 are `RESERVED`, so later searches (which only look
at `status = 'AVAILABLE'`) cannot see them. The database guarantees this,
so it stays correct even with many API servers.

**Spreading the load:** without an `ORDER BY`, every search reaches for the
same first rows, and concurrent searches keep skipping each other's locks.
Starting each search from a random point (`AND id >= $random`, wrapping
around if fewer than 5 are found) spreads users across the table.

---

## 5. After reserving: buy or release

A reservation lasts 5 minutes (`reserved_until`).

**Buy in time.** The buy query checks three things at once: the ticket is
still `RESERVED`, it is reserved by this user, and the time has not run out.

```sql
UPDATE tickets
SET    status = 'SOLD', sold_to = $user
WHERE  id = $ticket
  AND  status = 'RESERVED'
  AND  reserved_by = $user
  AND  reserved_until > now();
-- 1 row updated  -> success
-- 0 rows updated -> expired or not yours -> show "please search again"
```

**Don't buy.** A background job runs every 30 seconds and returns expired
reservations to `AVAILABLE`:

```sql
UPDATE tickets
SET    status = 'AVAILABLE', reserved_by = NULL, reserved_until = NULL
WHERE  status = 'RESERVED' AND reserved_until < now();
```

A purchase after the 5 minutes fails even if this job has not run yet,
because the buy query checks `reserved_until > now()` itself.

**Anti-hoarding:** limit each user to a maximum number of active
reservations (e.g. 10) and rate-limit the search API.

---

## 6. Performance

| Operation | How it is found | Expected time (10M rows) |
| --- | --- | --- |
| Prefix pattern `123***` | Range scan on the `number` index | < 5 ms |
| Mixed pattern `****23`, `1****5` | Digit indexes (BitmapAnd) or an early-stopping scan | ~10–50 ms |
| Reserve 5 tickets | Same query, `LIMIT 5`, locks only 5 rows | included above |
| Buy | Update by primary key | < 2 ms |
| Expiry job | Small update every 30 s | negligible |

*These times are estimates, not benchmark results.*

This handles **hundreds of searches per second** on a normal database
server. Read replicas can serve "browse only" traffic.

---

## 7. Trade-offs and scaling further

| Pros | Cons |
| --- | --- |
| One database, easy to build and operate | Mixed patterns cost tens of ms, not microseconds |
| Correctness is guaranteed by the database | On a big sale day, popular patterns (e.g. `****99`) concentrate load on one table |
| Easy to explain, test and debug | Reservations add write traffic |

**If traffic becomes very high** (e.g. thousands of searches per second when
sales open), keep PostgreSQL as the source of truth and move only the hot
path to **Redis**:

1. Keep one Redis set per number holding the available ticket IDs. `SPOP`
   removes a ticket from the set atomically, so two users can never take
   the same one.
2. Run search and reserve in one Lua script so they are atomic.
3. The final purchase still uses the PostgreSQL `UPDATE ... WHERE status`
   check, so a ticket can never be sold twice, even if Redis fails.

---

## 8. Summary

| Requirement | Solution |
| --- | --- |
| 10M tickets, 6-digit numbers | One PostgreSQL table + 6 generated digit columns |
| Wildcard search | Pattern → `WHERE d5='2' AND d6='3'`; partial digit indexes + BitmapAnd; prefix → range scan |
| No duplicate tickets at the same time | Search **reserves** with `FOR UPDATE SKIP LOCKED` in one statement |
| Fair release | 5-minute reservation; expiry job returns unused tickets |
| No double sale | Buy = `UPDATE ... WHERE status='RESERVED' AND reserved_by=user AND reserved_until > now()` |
| Performance | Index filtering + `LIMIT 5`; ms-level; random start point spreads load; Redis layer for peak traffic |
