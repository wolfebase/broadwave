-- Two waiting pairings must not share a code. A race between the check and
-- the insert used to store both; approve would then match the first one.
UPDATE pairings SET state = 'expired'
WHERE state = 'pending' AND id NOT IN (
	SELECT MIN(id) FROM pairings WHERE state = 'pending' GROUP BY code_hash
);

CREATE UNIQUE INDEX pairings_pending_code ON pairings (code_hash) WHERE state = 'pending';
