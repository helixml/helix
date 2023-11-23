```bash
ssh luke@node05.lukemarsden.net
cd ~/pm/helix
export WITH_RUNNER=1
./stack start
```

This will run the stack with the runner enabled.

In a different terminal:

```bash
sudo ssh -L 80:localhost:80 luke@node05.lukemarsden.net
ssh -L 8081:localhost:8081 luke@node05.lukemarsden.net
```

Now you can open http://localhost in your browser.