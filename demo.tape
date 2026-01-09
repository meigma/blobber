# Blobber CLI Demo
# Records a demonstration of core blobber functionality
# Run with: vhs demo.tape

Output demo.gif

Set Shell bash
Set FontSize 15
Set Width 900
Set Height 400
Set Theme "Dracula"
Set TypingSpeed 25ms
Set WindowBar Colorful

# Setup (hidden) - create sample directory with larger files for visible progress
Hide
Type "mkdir -p demo-config"
Enter
Sleep 200ms
Type "dd if=/dev/urandom of=demo-config/data1.bin bs=1M count=1 2>/dev/null"
Enter
Sleep 500ms
Type "dd if=/dev/urandom of=demo-config/data2.bin bs=1M count=1 2>/dev/null"
Enter
Sleep 500ms
Type "echo 'database: postgres' > demo-config/db.yaml"
Enter
Sleep 200ms
Type "clear"
Enter
Sleep 500ms
Show

# Demo starts
Type "# Push a directory to an OCI registry"
Sleep 500ms
Enter
Sleep 300ms

Type "blobber push --progress=tty ./demo-config ttl.sh/blobber-demo:v1"
Enter
Wait@30s

Sleep 1s
Enter
Sleep 300ms

Type "# List files without downloading the image"
Sleep 500ms
Enter
Sleep 300ms

Type "blobber ls -lH ttl.sh/blobber-demo:v1"
Sleep 500ms
Enter
Wait@10s

Sleep 1s
Enter
Sleep 300ms

Type "# Stream a single file to stdout"
Sleep 500ms
Enter
Sleep 300ms

Type "blobber cat ttl.sh/blobber-demo:v1 db.yaml"
Sleep 500ms
Enter
Wait@10s

Sleep 1s
Enter
Sleep 300ms

Type "# Copy a file from the image to local disk"
Sleep 500ms
Enter
Sleep 300ms

Type "blobber cp ttl.sh/blobber-demo:v1 db.yaml ./local-db.yaml"
Sleep 500ms
Enter
Wait@10s

Sleep 500ms

Type "cat ./local-db.yaml"
Sleep 500ms
Enter
Wait@5s

Sleep 2s

# Cleanup (hidden)
Hide
Type "rm -rf demo-config local-db.yaml"
Enter
Show
