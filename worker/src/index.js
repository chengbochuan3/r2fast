// r2fast expiry Worker — deletes objects whose `expire-at` metadata has passed.
//
// It scans only the EXPIRE_PREFIX namespace and reads custom metadata inline
// (R2 list supports include: ["customMetadata"]), so there's no per-object HEAD.
// Deletion uses the R2 bucket binding — no API token is involved.

export default {
  async scheduled(event, env, ctx) {
    const prefix = env.EXPIRE_PREFIX || "e/";
    const nowMs = Date.now();
    let cursor;
    let scanned = 0;
    let deleted = 0;

    do {
      const listed = await env.BUCKET.list({
        prefix,
        cursor,
        limit: 1000,
        include: ["customMetadata"],
      });

      const due = [];
      for (const obj of listed.objects) {
        scanned++;
        const exp = obj.customMetadata && obj.customMetadata["expire-at"];
        if (exp && Number(exp) * 1000 <= nowMs) {
          due.push(obj.key);
        }
      }
      if (due.length) {
        await env.BUCKET.delete(due); // R2 binding deletes up to 1000 keys at once
        deleted += due.length;
      }

      cursor = listed.truncated ? listed.cursor : undefined;
    } while (cursor);

    console.log(`r2fast-expiry: scanned ${scanned}, deleted ${deleted}`);
  },
};
