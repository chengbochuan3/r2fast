-- r2fast droplet: drag files onto this app to upload them to R2.
-- The download link is copied to your clipboard and shown as a notification.
-- Requires the `r2fast` CLI on your PATH (or in /usr/local/bin) and a config
-- created with `r2fast config init`. TTL comes from your config's default_ttl.

property binCandidates : {"/usr/local/bin/r2fast", "/opt/homebrew/bin/r2fast"}

on run
	display dialog "Drag files onto this app's icon to upload them to R2." buttons {"OK"} default button 1 with title "r2fast"
end run

on open theFiles
	set bin to findBinary()
	if bin is "" then
		display dialog "Couldn't find the r2fast binary." & return & "Install it on your PATH or in /usr/local/bin (see README)." buttons {"OK"} default button 1 with icon stop with title "r2fast"
		return
	end if

	set links to {}
	repeat with f in theFiles
		set p to quoted form of (POSIX path of f)
		try
			set theURL to do shell script bin & " upload " & p & " --no-copy"
			set end of links to theURL
		on error errMsg
			display dialog "Upload failed:" & return & (POSIX path of f) & return & return & errMsg buttons {"OK"} default button 1 with icon stop with title "r2fast"
		end try
	end repeat

	if (count of links) > 0 then
		set AppleScript's text item delimiters to linefeed
		set linkText to (links as text)
		set the clipboard to linkText
		display dialog "Uploaded · link copied to clipboard:" & return & return & linkText buttons {"OK"} default button 1 with title "r2fast"
	end if
end open

on findBinary()
	repeat with c in binCandidates
		set cpath to contents of c
		if (do shell script "test -x " & quoted form of cpath & " && echo yes || echo no") is "yes" then
			return quoted form of cpath
		end if
	end repeat
	try
		set found to do shell script "/bin/zsh -lc 'command -v r2fast' 2>/dev/null"
		if found is not "" then return quoted form of found
	end try
	return ""
end findBinary
