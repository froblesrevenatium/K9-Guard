SELECT
    i.code,
    m.itineraryid,
    m.serviceid,
    m.amount,
    count(*) AS adjustments,
    min(m.createddate) AS firstadjustment,
    max(m.createddate) AS lastadjustment
FROM
    @schema.mastertransaction AS m
    INNER JOIN @schema.itinerary AS i ON i.id = m.itineraryid
WHERE
    m.transactiontype = 3
    AND m.transactionsubtype = 0
    AND round(m.amount::numeric, 2) <> 0
    AND m.createddate > @since
GROUP BY
    i.code,
    m.itineraryid,
    m.serviceid,
    m.amount
HAVING
    count(*) > 1
    AND max(m.createddate) - min(m.createddate) < INTERVAL '1 minute'
ORDER BY lastadjustment DESC
