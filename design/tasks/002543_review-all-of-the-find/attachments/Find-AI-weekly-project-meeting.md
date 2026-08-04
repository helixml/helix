# Find-AI weekly project meeting

- **Date:** Thu, 25 Jun 2026 13:20:00 UTC
- **Participants:** luke@helix.ml, chris@helix.ml, leah.smith@linuxrecruit.co.uk, tony.chapman@linuxrecruit.co.uk, luke@mlops.consulting
- **Source:** fireflies

## Summary

- **Platform Onboarding:** Team set up Helix platform with shared access, Kanban task system, and AI-driven website update workflow.  
- **AI Agent Usage:** Agents use virtual desktops for independent frontend/backend tasks, accepting natural language and voice commands.  
- **Credit Management:** Each user has $100 monthly credits; costs tracked and saved with Claude AI or company GPUs to avoid overspending.  
- **Training & Support:** Live demos covered task workflows; ongoing help via Slack and future training planned to aid adoption.  
- **Strategic Vision:** Agents reduce developer bottlenecks, speeding updates and expanding into LinkedIn outreach and recruitment automation.  
- **Change Control:** GitHub integration and preview/comment/rollback features ensure safe, auditable changes with user approval.

## Transcript

Chris Sterry: Hi.
Luke Marsden: How you doing?
Leah Smith: I'm good.
Leah Smith: I'm melting slowly.
Luke Marsden: I think we all are.
Luke Marsden: That's literally the word I was just using.
Luke Marsden: When I was texting with my brother, he was like, oh, this estate agent I'm trying to rent a house off has just closed their office.
Luke Marsden: It's like, yep, everyone's melting.
Luke Marsden: We're all melting.
Leah Smith: Oh, God.
Leah Smith: It's like I can't even.
Leah Smith: If you sit for still for too long, it's.
Luke Marsden: Yeah.
Leah Smith: Sweat, but like, you can't go outside.
Leah Smith: I don't.
Leah Smith: It's.
Leah Smith: I don't know what to do.
Leah Smith: I don't know what the answer is.
Luke Marsden: Get an air conditioning unit.
Luke Marsden: It's the only answer.
Leah Smith: I do have one of those.
Leah Smith: It's in my daughter's room.
Leah Smith: Yeah, so she's.
Leah Smith: She's sleeping at the moment.
Luke Marsden: Okay.
Leah Smith: Yeah, yeah.
Leah Smith: My dad's around, so he's looking after her.
Luke Marsden: But.
Leah Smith: Yeah, because they're.
Leah Smith: They're child minds are shut as well.
Leah Smith: It's just like.
Luke Marsden: Yeah, well, you've made the right decision to let her have the air condom.
Leah Smith: I do sleep in there.
Leah Smith: Don't.
Luke Marsden: Yeah, that's good.
Luke Marsden: Hey, Tony.
Chris Sterry: How you doing?
Tony: Yeah, good, thanks.
Tony: Yourself?
Luke Marsden: Good, yeah, doing great, thank you.
Tony: Just surviving.
Tony: Hey, are your kids still off school or.
Luke Marsden: Yeah, they are.
Luke Marsden: I've been juggling.
Luke Marsden: Juggling childcare.
Luke Marsden: So I was looking after them this morning, but I'm free for now.
Luke Marsden: Chris, how's it going?
Chris Sterry: How about you?
Luke Marsden: Good.
Luke Marsden: That's a rustic looking scene behind you.
Chris Sterry: I am now in here.
Chris Sterry: We're now in Split, Croatia.
Chris Sterry: Yeah, it's very rustic.
Leah Smith: Yeah.
Leah Smith: A castle or something.
Chris Sterry: Yeah, down a little.
Chris Sterry: I don't know.
Chris Sterry: Everything in Europe's old, so I can't even say it's old.
Luke Marsden: Amazing.
Tony: You've been out on a.
Tony: On a boat yet and split, I think.
Chris Sterry: Well, so we were.
Chris Sterry: We were.
Chris Sterry: We just spent a week in Hvar.
Tony: Oh, nice.
Tony: Yeah.
Chris Sterry: And so now we came over to Split.
Chris Sterry: We are gonna rent a boat to go out to some of the, like, minor islands for a day and things like that.
Tony: But.
Tony: Yeah.
Tony: Yeah, a couple years ago with work actually, and.
Luke Marsden: Oh, cool.
Tony: I think we went to Hvar, actually.
Tony: Yeah, I think we flew into Split, but yeah, went item hopping and.
Tony: Yeah, so it's unbelievable.
Luke Marsden: It's lovely around there.
Tony: Yeah.
Tony: So nice.
Leah Smith: Cool.
Luke Marsden: So, yes, we basically, since we last spoke, we've been working on getting.
Luke Marsden: Hosting websites.
Luke Marsden: Working really well in helix because you guys wanted to host a website, so to be fully transparent.
Luke Marsden: That's what we've been.
Luke Marsden: That's what we've been doing and that's now working.
Luke Marsden: So.
Luke Marsden: And that's also kind of pushed us in a useful product direction because agents being able to host their own websites is super powerful and it's something we're seeing other people doing as well.
Luke Marsden: So thank you.
Luke Marsden: And yeah, so we should.
Luke Marsden: What I wanted to do on this call, if it's okay, is get you set up actually using the platform so that you can now make changes to your own website by using the agents to change the website.
Tony: Okay.
Luke Marsden: And that should be an interesting, interesting test.
Luke Marsden: And then once that's.
Luke Marsden: And then once that's.
Luke Marsden: Once we've had some time to work on that together, resolve any issues that come up, whatever.
Luke Marsden: If you need support with any of the changes you want to make, then I can jump on and help with that.
Luke Marsden: And then after that I'm going to.
Luke Marsden: We can.
Luke Marsden: When you're happy with the website, we can then put it live.
Luke Marsden: I've got your 1, 2, 3 REG login.
Luke Marsden: And then I can switch modes to focusing on the agents, the Jack and Jill stuff, the internal productivity things.
Luke Marsden: Is that okay with everyone?
Leah Smith: That's good, yeah.
Luke Marsden: Okay, awesome.
Luke Marsden: Great.
Luke Marsden: Thank you.
Luke Marsden: So let me.
Luke Marsden: I know you both just registered.
Luke Marsden: Let me just check something on my system here.
Chris Sterry: I approved him, Luke.
Luke Marsden: Okay, great.
Luke Marsden: Ah, cool.
Luke Marsden: Yeah, I was gonna activate a trial as well.
Luke Marsden: So it's not really a trial.
Luke Marsden: How we call that.
Luke Marsden: Cool.
Luke Marsden: So I've given you both some credit as well in system.
Luke Marsden: So yeah.
Luke Marsden: Who wants to share their screen first?
Luke Marsden: Let's.
Luke Marsden: Tony or Leah.
Luke Marsden: We can.
Luke Marsden: If you want to share your screen, I can kind of walk you through logging in.
Luke Marsden: Oh and we should probably.
Luke Marsden: Okay, great.
Luke Marsden: Yeah.
Luke Marsden: So if you, if you hear from.
Tony: The, from the email.
Luke Marsden: Yeah, yeah, yeah.
Luke Marsden: So hit refresh on that just to double check that it's.
Luke Marsden: Yeah, that's fine.
Luke Marsden: Okay cool.
Luke Marsden: So yeah if you could make a new organization, just call it Find AI.
Tony: Well let me select that.
Luke Marsden: Yeah, that's interesting.
Luke Marsden: UX feedback.
Luke Marsden: People always try and click on the little circle.
Luke Marsden: So that's something we need to.
Luke Marsden: We need to fix that.
Luke Marsden: I mean it needs to make it less obvious that.
Tony: Yeah, I see now that's.
Tony: I've done that.
Tony: So that's ticked and then want to do.
Luke Marsden: Yeah, I see, I know but everyone always clicks on the circle because it looks like a clickable thing.
Luke Marsden: So we should fix that.
Luke Marsden: That's fine.
Luke Marsden: Ignore the fact that it's called a trial.
Luke Marsden: I just set you up with a trial for a.
Luke Marsden: A year because that was.
Luke Marsden: And I'll just renew it whenever we need to.
Luke Marsden: So just hit continue and then you can ignore that.
Luke Marsden: Don't.
Luke Marsden: You don't need to connect anything.
Luke Marsden: You've got all of those globally.
Luke Marsden: Just hit continue on it.
Luke Marsden: Yeah, that's fine.
Luke Marsden: And then you can create a project if you scroll up a little bit.
Luke Marsden: Oh, sorry.
Chris Sterry: You have to project name before you can do anything.
Luke Marsden: No, I know, but I'm thinking I've made a project and I want to hand it over to you, so.
Luke Marsden: Well, yeah, just make a new project.
Luke Marsden: Just call it test.
Luke Marsden: Just.
Luke Marsden: That could just be like a little test project that we use to.
Tony: I'll be with the rest of it.
Luke Marsden: That all looks fine.
Luke Marsden: Yeah, thank you.
Luke Marsden: Thanks for checking.
Luke Marsden: And then just write test because I'm going to kind of hand you something rather than.
Luke Marsden: Okay, so now.
Luke Marsden: Now you're in helix and let me just kind of.
Luke Marsden: This can be a little bit of a training session for you guys.
Luke Marsden: So everything in helix is organized in a Kanban board.
Luke Marsden: I'm sure you're familiar with Kanban boards, right?
Luke Marsden: You probably use them the process stuff and recruiting as well.
Luke Marsden: I don't know.
Luke Marsden: But basically every task that you get an agent to do in here will start in the backlog.
Luke Marsden: You can hit plan.
Luke Marsden: You can start the planning.
Luke Marsden: The agent will come up with a plan.
Luke Marsden: You can then iterate on the plan with it.
Luke Marsden: It's a bit like a Google Docs like interface for commenting on the plan.
Luke Marsden: And then once you've approved the plan that it will go and do the work.
Luke Marsden: And then when it's done, it will open a pull request and that pull request allows you to then approve the change and get it deployed.
Luke Marsden: So we'll use this process and you can.
Luke Marsden: Yeah, hit start planning on test.
Tony: Are you recording this, Luke, by any chance?
Luke Marsden: It is on Fireflies.
Luke Marsden: Yeah.
Luke Marsden: So I can share the Fireflies.
Tony: Okay, sweet.
Luke Marsden: Yeah, yeah, yeah, I'll share the Fireflies.
Tony: Just so I can show some of the guys as well.
Luke Marsden: Oh, yeah, of course.
Luke Marsden: I think the Fireflies screen share is a little bit grainy, but it should be fine.
Luke Marsden: Yeah, yeah, yeah.
Chris Sterry: And if we need to, I'm happy to jump on a call with the rest of your team too.
Chris Sterry: We can walk through this again if we need to.
Luke Marsden: So yeah, yeah.
Luke Marsden: Thank you for some more training with you.
Luke Marsden: But yeah, so yeah, just try start planning there.
Luke Marsden: And this will probably do something silly like.
Luke Marsden: Oh, we're just verifying that the system works because you just wrote test.
Luke Marsden: But normally if you write something more interesting then then yeah.
Luke Marsden: And the idea with helix is you give each agent its own computer.
Luke Marsden: And so what you can see inside that little box is the agent's computer.
Luke Marsden: It's like an actual desktop environment.
Luke Marsden: You can click on it and it's like a computer inside your browser basically.
Luke Marsden: And each agent here is given its own ide, like its own development environment.
Luke Marsden: But you don't really need to overthink that because you can just chat to it on the left and you can tell it to do things like open the app in a browser and make the.
Luke Marsden: Make this button green instead of yellow or whatever you need to do.
Luke Marsden: And it.
Luke Marsden: And it will just kind of do the right thing.
Luke Marsden: So yeah, if we leave that running, I think the next most important thing is to make sure that we can get Leah to join.
Luke Marsden: Actually, before you stop sharing your screen, Tony, would you please go to.
Tony: This has just popped up by the way.
Tony: Do I just.
Luke Marsden: Yeah, that's fine.
Leah Smith: Just.
Luke Marsden: Just ignore it for now.
Luke Marsden: I mean basically that's the plan and it's decided to add a project, add a readme file and it's like fine.
Luke Marsden: Okay, but if you go settings bottom left.
Luke Marsden: Thank you.
Luke Marsden: To the org settings and then you can find people on the left.
Luke Marsden: Would you mind adding me to this org so that we can collaborate?
Luke Marsden: Is that okay?
Luke Marsden: So if you search for loot, I'm Luca Mlops on here.
Luke Marsden: There's actually loads of me in system because I test it all the time.
Luke Marsden: But Luke at ML, That's it, that's my main account on there.
Luke Marsden: And if you wouldn't mind just switching me to an owner just so I have full access, that will help.
Luke Marsden: Thank you.
Luke Marsden: Awesome.
Luke Marsden: So now Leah, if you want to share your screen.
Luke Marsden: Oh no, actually Leah's already in the system so Sorry Tony, if you could share your screen again, we can probably find Leah already.
Luke Marsden: Thank you.
Leah Smith: Do I need to after the call do what Tony just done and obviously create the organization and always.
Luke Marsden: No, because we're going to add you to the same org.
Luke Marsden: So we'll get to that in a second.
Luke Marsden: So I'll get you to share your screen in a second Lear and we'll set you up as well.
Leah Smith: Well.
Luke Marsden: So there you go, owner.
Luke Marsden: Awesome.
Luke Marsden: Okay, so now yeah, Leah, if you wouldn't mind sharing your screen please.
Luke Marsden: Cool, thank you both.
Luke Marsden: Great to get you on boarded.
Tony: It's exciting stuff.
Luke Marsden: Yeah, yeah.
Luke Marsden: Okay, awesome.
Luke Marsden: So ah, you made another org.
Luke Marsden: Okay, that's fine.
Leah Smith: I tried to be clever and.
Chris Sterry: Okay.
Luke Marsden: If you click top left actually if you hit refresh first because it might just need a refresh because you've got added.
Luke Marsden: Yeah.
Luke Marsden: And then click top left on the F. There you go.
Luke Marsden: You can see the two Find AI.
Luke Marsden: So you can go into the one that Tony made for you, which is the original one.
Luke Marsden: You might want to go back.
Luke Marsden: And then you can see all the work he's doing.
Luke Marsden: So that's like a collaborative thing.
Luke Marsden: You can both have access to the same Kanban board, you can both look at the same agents.
Luke Marsden: It might, to avoid confusion, be worth you going back to the other one you set up.
Luke Marsden: And just in the settings, bottom left.
Luke Marsden: Yeah, just there.
Luke Marsden: Oh, sorry, not account settings.
Luke Marsden: Just set it like it's the.
Luke Marsden: Org settings.
Leah Smith: Sorry, bear with.
Luke Marsden: No, no, it's fine.
Luke Marsden: You can just hit X on that and the little cog icon in the bottom left.
Luke Marsden: Yeah, yeah, just put like old in brackets after the name.
Luke Marsden: Find AI.
Luke Marsden: Yeah.
Luke Marsden: And then hopefully it will show up.
Luke Marsden: Yeah, it shows up as old.
Luke Marsden: So now you know which one to go to.
Luke Marsden: So that should be fine.
Luke Marsden: Cool.
Luke Marsden: So now I need to transfer my.
Luke Marsden: Oh, can you still hear me?
Luke Marsden: Yeah, okay, sorry, my zoom confused me.
Luke Marsden: Let me just see if I can transfer this one.
Luke Marsden: I set up over to you.
Luke Marsden: I am predictably asking Claude to help me do that.
Tony: So is it.
Tony: Is this the.
Tony: This is the agent.
Tony: So we essentially like set up a task and say I would like you to do this and then.
Luke Marsden: Yeah, and we're going to start with using the agents to help you update your own website.
Luke Marsden: Okay, I think that's that.
Luke Marsden: Because the website is kind of the first deliverable, isn't it?
Luke Marsden: Yeah, yeah.
Luke Marsden: So rather than just like acting like an old school web developer where I'm like, yeah, tell me what you want and we'll talk instead, I'm going to be like, tell the AI what you want and if you don't need my help, then I'll help you.
Luke Marsden: If that's okay.
Tony: Yeah, because then we can make changes anytime then, can't we?
Luke Marsden: Well, exactly.
Luke Marsden: And you can even add new features and stuff.
Luke Marsden: If you do anything like sort of quite complicated, then I'm happy to review it because you are like shipping code to production at that point, but for like copy changes, updating images, all that stuff.
Luke Marsden: This would be.
Luke Marsden: This is like an alternative to a cms basically.
Luke Marsden: Yeah.
Leah Smith: If you want to add a new.
Leah Smith: So say the homepage, for example, you want to add in like a carousel for jobs or client logos and stuff like that.
Leah Smith: And you send.
Leah Smith: You provide the AI agent with like screenshots of things that you like.
Leah Smith: So is it able to do that?
Luke Marsden: Literally exactly that.
Luke Marsden: Yeah, yeah, yeah, yeah.
Tony: We were playing around with replit for a bit.
Tony: Yes, a bit like that kind of thing.
Luke Marsden: Exactly.
Leah Smith: Yeah.
Tony: Ask it.
Tony: And if you don't like it, you can go back and to a previous step or you can ask it to differently, whatever.
Luke Marsden: Yeah, exactly.
Luke Marsden: It's like replet on steroids because it can also do, like, backend stuff, databases.
Luke Marsden: Amazing.
Luke Marsden: And.
Luke Marsden: And yeah, you can also, like, preview what the agent is doing, like in its own desktop, because it can launch its own browser in its own desktop and then you can look at that.
Luke Marsden: So you can kind of see live what it's doing.
Luke Marsden: And also QA with it, which is quite nice.
Luke Marsden: You can click around.
Luke Marsden: It can click around.
Tony: Yeah.
Chris Sterry: Literally, while we're on this call, having IT do prospecting for me in LinkedIn, it's running as we're talking.
Luke Marsden: Yeah, yeah, yeah, that's cool.
Tony: Yay.
Tony: Yeah, I think.
Tony: Yeah, definitely that side of things.
Tony: We're really keen on candidate, you know, outreach and BD outreach as well.
Luke Marsden: Yeah, nice.
Luke Marsden: So let me just do this.
Luke Marsden: If this doesn't work, I'll.
Luke Marsden: We can just set it up again.
Luke Marsden: But I'd rather just transfer the existing project over.
Luke Marsden: I should have thought about this before, but this is why we're doing it now.
Luke Marsden: How are you guys for time, by the way?
Luke Marsden: I know you had to start late.
Tony: I've got it at three, so that's fine.
Luke Marsden: Oh, yeah, you got it to your school run?
Tony: Yeah, only because my wife.
Tony: My wife's gone to.
Tony: She's gone away with some friends.
Tony: To where she gone?
Tony: Mallorca for a few days, which isn't ideal because the kids school, one of them is closed, so I've got to look after my daughter and then my son.
Chris Sterry: I think it sounds ideal for her.
Tony: Oh, yeah, for her, yeah.
Leah Smith: Yeah,.
Tony: She's having a great time.
Tony: Yeah, sorry.
Tony: Yeah, so I've probably got till 3, 5, 3, something like that.
Luke Marsden: Yeah, that's absolutely fine.
Luke Marsden: Yeah, there's no stress.
Luke Marsden: I'm just glad we got to connect because I had to reschedule this so many times.
Luke Marsden: So.
Luke Marsden: Thank you.
Luke Marsden: So,.
Leah Smith: Yeah, sorry, is there a limit on what you can ask the agent?
Leah Smith: So do we have, like, you say a certain amount of credits or do certain things use a certain amount of credits, depending on how big.
Luke Marsden: Yeah, they do.
Luke Marsden: So if you go to.
Luke Marsden: And I've given you.
Luke Marsden: I think I gave you 100 bucks each.
Luke Marsden: $100 Each as a starting point, which kind of comes out of the 499 that you paid.
Luke Marsden: So if you Go to.
Luke Marsden: I think if you go to Settings.
Luke Marsden: Oh no, if you actually, if you go.
Luke Marsden: If you click the little helix icon in the bottom left.
Leah Smith: Yeah.
Tony: Do you want to give that a go?
Luke Marsden: Clear.
Leah Smith: Yeah.
Leah Smith: Am I still sharing what else?
Luke Marsden: I didn't know.
Luke Marsden: No, just the one tab, I think.
Tony: I tried clicking on it but.
Leah Smith: So where am I going?
Leah Smith: Sorry, the.
Luke Marsden: The helix icon in the bottom left and then go Account Settings.
Luke Marsden: I'm actually not entirely sure where this is.
Luke Marsden: Oh yeah.
Luke Marsden: But your spend I think shows up here.
Luke Marsden: Okay.
Luke Marsden: So you can see how much you've spent on, on tokens.
Luke Marsden: And when credit runs out, like when the $100 runs out, then it will just give you an error message and you can top up with a card.
Luke Marsden: But yeah, so we got that set.
Tony: As a limit at the moment of 100.
Luke Marsden: Yes, it's a limit of 100.
Luke Marsden: Of 100 bucks.
Tony: Do you think that should last us for most months or is that $100 a month?
Tony: Sorry.
Tony: Or is that.
Chris Sterry: And to be clear, it's a limit of $100 a month that we're giving you in tokens.
Chris Sterry: You can have as much as you want beyond that, but that's part of that, that 499 got you.
Tony: Will that typically last us for.
Luke Marsden: Yeah, it should.
Luke Marsden: It should last you a while.
Luke Marsden: And you can also.
Luke Marsden: Do you have a Claude subscription?
Tony: I pretty sure I do.
Tony: Yeah.
Tony: I don't use it.
Tony: I don't use it that much but I'm pretty sure I've got one.
Luke Marsden: Okay.
Luke Marsden: Yeah, because if you've already got Claude subscription, you can actually plug that in and then just eat out of the Claude subscription, if that makes sense.
Luke Marsden: Which will then just use up your usage within that subscription.
Luke Marsden: But if you're not using it much then that might be fine.
Luke Marsden: So yeah, that's cool.
Luke Marsden: And we've also got models that we run on our own GPUs that are a lot cheaper and we've been testing them ourselves and getting really good results.
Luke Marsden: So that's another option as well.
Luke Marsden: So there's various different ways of making it not too expensive.
Leah Smith: So things that typically use up a lot of credits just so that we're aware of what things like might use quite a lot.
Luke Marsden: Yeah, I think it's like, I mean, honestly the kind of changes you're likely to make to a front end website are not likely to use up a lot of credits.
Tony: Yeah.
Luke Marsden: I think if you did like drag in like really massive PDFs or something that had loads of content, then that might chew through a bit.
Luke Marsden: I don't know if you'd do that.
Luke Marsden: You probably just upload like a couple of images, like screenshots and be like, make it like this.
Luke Marsden: Or go look at this website and copy.
Luke Marsden: Make it look a bit like that one, because I like it.
Luke Marsden: Pasting in a bit of copy that you've written, asking it to refine it, add a blog post or whatever like that.
Luke Marsden: All that stuff will be fairly cost effective even with the Claude API that you're using here.
Luke Marsden: So cool.
Luke Marsden: So if you go back to projects now, this is where I have to cross my fingers because I just got Claude to like edit the database live.
Luke Marsden: Okay.
Luke Marsden: Yeah.
Luke Marsden: So you can see find AI in there.
Luke Marsden: Tony maybe confirming that you see it as well.
Luke Marsden: Sorry.
Luke Marsden: It's fine.
Luke Marsden: You should.
Luke Marsden: Do you hit refresh?
Tony: Yeah, I got that as well.
Luke Marsden: Cool.
Luke Marsden: So if you click into there, please, Leah.
Luke Marsden: Yeah, yeah.
Luke Marsden: So you should be able to go in there and click on the desktop.
Luke Marsden: I just put a simple little test in there.
Luke Marsden: Oh, and you can see Tony's in there.
Tony: But yeah, yeah, yeah, yeah, yeah.
Luke Marsden: So now you're co piloting,.
Tony: So.
Luke Marsden: So that's cool.
Luke Marsden: And that was just a.
Luke Marsden: A test to check that the website's up in the local dev stack.
Luke Marsden: So basically each of these tasks gets its own computer for the agent, and that agent can make its own changes that won't interfere with the other changes that the other agents are making, if that makes sense.
Luke Marsden: So they won't like tread on each other's toes.
Luke Marsden: And none of the changes you make will go live until after you actually click the button for Open pr.
Luke Marsden: PR means pull request and then merge that pull request on GitHub.
Luke Marsden: So that's like the flow, basically.
Luke Marsden: So now you're web developers.
Chris Sterry: Sorry,.
Tony: What do we have to do to make changes?
Tony: Just type in there and click Enter.
Tony: And that's literally it.
Luke Marsden: So let's do a test one.
Luke Marsden: So Leah, if you go back to the project, you could carry on this session, but I would make a new task for every change you want to make.
Tony: Right.
Tony: Okay.
Tony: You don't just do it every time from here.
Luke Marsden: No, you could reuse the same session, but it's nicer to keep them organized as like tickets, basically.
Tony: So let's say we add like an event on there just for.
Tony: Because that's probably what.
Tony: What we mostly do.
Tony: Yeah, an event.
Tony: And then in a month's time you want to add another event.
Tony: Would you start that as a new task or would you go into the.
Tony: Almost the events kind of task?
Luke Marsden: You keep going back I would make a new event as in, yeah, sorry, new task for every new thing you want to do and kind of keep them short lived because it's like you're going to like march it through the different columns and then it'll be finished when it's in the right hand most column.
Luke Marsden: So the other thing I would say is let's actually just have a quick look at the website because I posted a link to the latest staging site on Slack earlier.
Luke Marsden: So I don't know Leah, if you want to click on that on Slack and then switch over to it.
Luke Marsden: Oh, thank you.
Luke Marsden: Yeah.
Luke Marsden: So take a look through here and just pick some small change you want to make to begin with.
Luke Marsden: Like the, the other, the other page.
Luke Marsden: All of the pages are fairly minimal at the moment, so I don't know if you want to like add some stuff to them.
Leah Smith: Something that's not too a big of a task I reckon.
Leah Smith: Tony, what do you think?
Tony: I could do anything really to just.
Luke Marsden: And I'll work on the big things like the actual job search and making that work against your database.
Luke Marsden: So I'm not expecting, I'm not handing that work over to you, but this is more like the content stuff.
Tony: Did we, we agreed to remove the Insights tab, didn't we, last time?
Luke Marsden: Yes.
Luke Marsden: That's a good one.
Luke Marsden: Love that.
Luke Marsden: Yeah.
Luke Marsden: This is just a good way to kind of get used to the flow.
Leah Smith: So is it here?
Luke Marsden: Just say remove the Insights tab and.
Leah Smith: Then does it matter what normally.
Leah Smith: Okay.
Luke Marsden: Yeah.
Leah Smith: Is it.
Leah Smith: I'm typing in capital.
Leah Smith: Sorry, it doesn't mind me.
Luke Marsden: You can shout at it.
Leah Smith: Now.
Leah Smith: Remove Insights tab.
Leah Smith: And that's literally.
Chris Sterry: And just to give you some context of how I use it, I think Luke does too.
Chris Sterry: Like I'll just talk to it.
Chris Sterry: I'll turn on my, my.
Chris Sterry: I'm on a Mac, right.
Chris Sterry: So I hit the, the microphone button and I'll, I'll speak naturally to it and say I no longer want to have an Insights tab on the website.
Chris Sterry: You know, I want to do X, Y and Z and I just kind of like free flow it and then hit enter and so you don't have to be as deliberate as you.
Chris Sterry: You think you do.
Luke Marsden: One thing I noticed quickly is that when I moved it over it hasn't kept the agent.
Luke Marsden: So let me just go in here before you hit go, please.
Luke Marsden: I just want to make sure that we wire that up properly.
Chris Sterry: Sorry Tony, what was your question?
Tony: I was just going to say you talked it.
Tony: Can you talk to it through your laptop?
Chris Sterry: Talk to me on My laptop, I use a separate program just because I like the.
Chris Sterry: The way it.
Chris Sterry: It translates better.
Chris Sterry: I use a product called Whisper Flow,.
Luke Marsden: But for a long time, you guys with Mac.
Luke Marsden: Oh, perfect.
Luke Marsden: Yeah.
Luke Marsden: I really recommend Whisper Flow.
Luke Marsden: Then, like Chris is saying, but for.
Chris Sterry: A long time, I just use Mac dictation.
Chris Sterry: Like, in all honesty, it's just.
Tony: It.
Leah Smith: The.
Chris Sterry: The benefit of Mac dictation is you can see what you're writing as you're writing it.
Chris Sterry: Where Whisper Flow batches everything.
Chris Sterry: But I find that Mac dictation often miswords things.
Chris Sterry: And so you wind up using your keyboard to go, like.
Chris Sterry: As you see it working forward, you're like, oh, that's not what I said.
Chris Sterry: And then you'll stop and then kind of edit it and then go back to it.
Chris Sterry: Where Whisper Flow, I think, does a much better job of just catching everything, but it doesn't show you until you click the button again.
Chris Sterry: So I just.
Chris Sterry: I have a shortcut on my keyboard.
Chris Sterry: I also have a shortcut on my mouse that toggles my.
Chris Sterry: My microphone.
Chris Sterry: And then I just talk to it and it works really well.
Luke Marsden: And I do the same thing.
Tony: If you go back up to that box.
Tony: Leah.
Leah Smith: Yeah.
Tony: How would we.
Tony: How would we.
Chris Sterry: So on your Mac keyboard, F5, you've got the microphone button.
Tony: Oh, yeah.
Chris Sterry: Tap microphone and start talking.
Chris Sterry: No pressure.
Chris Sterry: Here you go.
Leah Smith: So do you have to keep your hand on the F5 and the tap just.
Chris Sterry: You should just be able to tap it and then when you're done, tap it again.
Chris Sterry: I believe this is how that works.
Leah Smith: Remove Insights tab.
Luke Marsden: See, if it were.
Chris Sterry: Now that I'm on Whisper Flow, I don't know.
Chris Sterry: I don't recall what the default max setting is.
Tony: Technically illiterate.
Tony: We are.
Tony: Didn't even know that was a thing.
Luke Marsden: Yeah, that when I press that button on my keyboard, it tries to enable dictation.
Luke Marsden: So I think we need to fiddle with the settings.
Luke Marsden: But maybe this is a bit of a rabbit hole.
Luke Marsden: There we go.
Luke Marsden: That's.
Luke Marsden: But there'll be a way to set up dictation on your Mac, which.
Luke Marsden: Which is actually a really nice way to talk to AI because you.
Luke Marsden: Yeah, we do that.
Luke Marsden: But I recommend Whisper Flow as well.
Luke Marsden: It's.
Luke Marsden: It's really nice.
Luke Marsden: It is a paid product and.
Luke Marsden: But it's the best one that I've used.
Luke Marsden: So under Agent, if you click Select Agent.
Luke Marsden: Oh, yeah, just.
Luke Marsden: Just click.
Luke Marsden: Just select Claude code.
Leah Smith: Yeah, I always select that one.
Tony: Yeah.
Luke Marsden: Yeah, just use that one.
Luke Marsden: And we can add some other ones.
Luke Marsden: For example, if we use, like, the cheaper Models.
Luke Marsden: We'll use a different agent but you don't need to think too much about that.
Leah Smith: Perfect.
Leah Smith: And then create task.
Luke Marsden: Yep.
Luke Marsden: And then hit start planning.
Chris Sterry: So what you can do is in that.
Chris Sterry: Oh, hang on a sec.
Luke Marsden: Can we just follow through on this?
Luke Marsden: Yeah.
Luke Marsden: So this is interesting because we need to set up the repo is currently still in our org.
Luke Marsden: Are you both logged in as Tony Chapman.
Leah Smith: Frog.
Luke Marsden: You probably are just fine.
Luke Marsden: I can just add Tony to.
Tony: Is that.
Tony: Is that my GitHub is it?
Luke Marsden: Yeah, yeah.
Luke Marsden: And I haven't yet moved your repo that we've made for you into your GitHub.
Luke Marsden: But we will.
Luke Marsden: But for now I'm just gonna add you.
Luke Marsden: Sorry, just get my phone to two flags.
Luke Marsden: Authentic.
Luke Marsden: People.
Luke Marsden: Tony chapman.
Luke Marsden: Wrong.
Luke Marsden: Yeah, I'm gonna give you.
Luke Marsden: Maintain permission on that.
Luke Marsden: Tony you or actually Leah because you're logged in as well as Tony.
Luke Marsden: There.
Luke Marsden: There should.
Luke Marsden: You should now find an invitation.
Luke Marsden: I just sent you a link.
Leah Smith: Is that to my gmail?
Luke Marsden: No, sorry.
Luke Marsden: On the Zoom chat I should have said Ah, perfect.
Tony: Let's come up with like a 404.
Tony: Is that because I need to log in?
Luke Marsden: Probably, yeah.
Luke Marsden: But you.
Luke Marsden: Leah's logged in as you so you can hit accept.
Luke Marsden: Okay cool.
Luke Marsden: So now you can access that.
Luke Marsden: So now if you go back to the other tab, the helix tab.
Luke Marsden: I wonder if you hit refresh here.
Luke Marsden: If you now see.
Luke Marsden: Yeah, okay, cool.
Luke Marsden: And you can also click before you click authorize.
Luke Marsden: Click Grant next to we find AI as well.
Luke Marsden: Okay, it's now going to ask Tony.
Luke Marsden: Tony, please give us your code.
Luke Marsden: Sorry.
Luke Marsden: Oh, this is fun though.
Luke Marsden: It's good to get you all wired in.
Tony: Yeah, really annoying.
Tony: I just logged into asked to verify my device.
Tony: You re you able to resend it?
Tony: Say resend there?
Tony: No.
Luke Marsden: Maybe you could just click the X and then click Grant again.
Tony: Yeah.
Tony: Oh, sorry.
Tony: I was checking the wrong email, wasn't I?
Luke Marsden: That's fine.
Tony: I've just logged into my GitHub as Linux recruit rather than.
Tony: Right, we've got it.
Luke Marsden: Okay, that's fine.
Luke Marsden: Yeah.
Luke Marsden: If you just check your WI fi and AI email.
Tony: There you go.
Luke Marsden: Oh, nice.
Luke Marsden: Okay, so you can copy paste that Leo.
Leah Smith: Ah, sorry, it's in the chat.
Luke Marsden: Awesome.
Luke Marsden: Why does it not let you authorize now?
Luke Marsden: That's weird.
Leah Smith: I'll try refresh.
Luke Marsden: Yeah, that's what I was going to say.
Leah Smith: There we go.
Luke Marsden: Okay.
Luke Marsden: Weird.
Chris Sterry: Yay.
Luke Marsden: Okay,.
Tony: Cool.
Luke Marsden: Right?
Luke Marsden: Yeah.
Luke Marsden: Start planning.
Luke Marsden: Interesting.
Luke Marsden: This should just take a second.
Chris Sterry: While that's doing that, I'll just say so as you put in, you Know, remove the, the link or whatever that was you want to remove from the navigation.
Chris Sterry: So, so the backlog here is you can just start as you're building things or as you're thinking about things, or as you guys have meetings.
Chris Sterry: You guys can start like talking about, hey, I want to go add this piece, I want to go add this piece, I want to go do those things.
Chris Sterry: And you can add those to the backlog.
Chris Sterry: And that just becomes your working kind of subject matter there.
Chris Sterry: So it hasn't started planning, it hasn't started operating on it.
Chris Sterry: But when you guys are just talking about, hey, I'd love to do this in the future, toss that into the backlog right now.
Chris Sterry: It'll just live there until you guys are ready to build that out.
Chris Sterry: And so it makes it really nice to just kind of play in.
Tony: Sorry.
Tony: So just if you've got ideas, you just.
Tony: Yeah, as soon as you think of them, chuck them in there.
Tony: Yeah, okay.
Luke Marsden: Exactly.
Luke Marsden: You can prioritize them and then they'll forward.
Luke Marsden: It'll be sorted by priority.
Luke Marsden: So, yeah, be like, oh, yeah, suddenly it's important to do this one.
Luke Marsden: And then you can set the agent off from doing it and get it over the line yourself.
Luke Marsden: Or I can help, like, if you get stuck with anything or if the agent gets stuck.
Luke Marsden: And yeah, you can even ask it to do things like build a, Build a new blog section or like build an events section because it's a very capable software engineer.
Luke Marsden: It's like you've got a team of software engineers at your beck and call now.
Luke Marsden: Kind of fun.
Luke Marsden: So.
Leah Smith: And is it easy to like Ripley if you want to, if you make a change and you want to go back or change, you know, you don't want to do what you've.
Leah Smith: Just ask it to do and like delete it.
Leah Smith: Is it easy to do that as well?
Luke Marsden: Yeah, so probably the easiest way to do that would just be to make a new task that is like revert that thing I just did.
Luke Marsden: And then that would.
Luke Marsden: It would do the.
Luke Marsden: Do the things that you need to do.
Luke Marsden: Or you can go into GitHub and go to your pull requests.
Luke Marsden: And I think GitHub has a button for like, unmerge the pull request.
Luke Marsden: It's like undo.
Luke Marsden: So you can also go through that path, but probably the easiest path is just to tell the AI to undo the thing we just did.
Luke Marsden: And if you take a look in the bottom right of each of these little tabs, there's a number.
Luke Marsden: So.
Leah Smith: Oh, yes.
Luke Marsden: So it's only barely visible above that blue button, but you can see the other one is called fight is number five of five.
Luke Marsden: So you can refer to those by number because all of them will that all of the, all of the agents can see the plans that all of the other agents had.
Luke Marsden: So it acts like a kind of working memory of all the changes.
Tony: Do we preview?
Tony: So if we, for instance, this one Remove Insights tab, does it show us a preview of what it would look like before say yeah, we accept that and push it to action.
Luke Marsden: Yeah, exactly.
Luke Marsden: And you can look at the preview just in the browser as in the agent's desktop.
Luke Marsden: And I'm also going to add a feature where each pull request gets owned.
Luke Marsden: Gets its own link, which is like a preview link.
Luke Marsden: So you can look at it in your own browser.
Luke Marsden: But.
Tony: Spec there.
Luke Marsden: Yeah, yeah, let's read the spec quickly.
Leah Smith: Yeah, this, the website spec.
Tony: Click Review spec here.
Leah Smith: Yeah.
Tony: And then it tells you what the, what it's going to do.
Tony: Basically.
Luke Marsden: It's kind of over overcooked this a little bit, but you get it.
Tony: Get does all that just from remove Insights down.
Luke Marsden: Yeah, exactly.
Luke Marsden: Because what it's done is it's looked at the code and your request and it's figured out all of the different like details about what it needs to do.
Tony: So we, we read this back and say, okay, cool, that is what I was asking it to do.
Luke Marsden: Yeah, yeah.
Luke Marsden: Or if you, if it got the wrong end of the stick.
Tony: Yeah.
Luke Marsden: Then if you hover over it, you'll see there's a comment button.
Luke Marsden: Or you can also highlight any text you like and leave a comment and then the agent will immediately respond to your comment and change the document in front of you.
Luke Marsden: So you can kind of steer it that way.
Leah Smith: Perfect.
Tony: Request changes.
Luke Marsden: You can also just do request changes which will then do.
Leah Smith: Yeah, yeah.
Luke Marsden: So it's probably worth testing that out when you actually have a change you want to make.
Luke Marsden: I think I got this one right.
Tony: So if you, I thought when you accept it, you, you request the changes that it's.
Luke Marsden: Oh no, sorry.
Luke Marsden: Request changes is request changes to the spec.
Luke Marsden: If you click next document.
Luke Marsden: That's kind of the.
Luke Marsden: Yeah, click next document.
Luke Marsden: The technical design, you can kind of assume that it will get it right.
Tony: Yeah.
Luke Marsden: On the technical design.
Luke Marsden: So you don't need to like really understand everything in here to be able to go ahead with it and then go next document one more time.
Luke Marsden: And this is the to do list.
Luke Marsden: Basically it's given itself a list of things that it needs to do to get this done.
Luke Marsden: You don't need to tick them off.
Leah Smith: Because it will take them off and then approve design.
Luke Marsden: Yeah.
Luke Marsden: And then hit approve.
Luke Marsden: And you don't need to leave a comment unless you want to.
Luke Marsden: And then it's going to go ahead and do the work.
Tony: Great.
Luke Marsden: And that really shouldn't take very long, so we can probably just watch it live.
Luke Marsden: But then the really cool thing is when you can then say show me in the chat and it will show you in the agent's own computer.
Leah Smith: Do you type that in there?
Luke Marsden: Yeah, yeah.
Luke Marsden: But wait, wait for it to finish.
Luke Marsden: I think, I mean, I think it's basically finished now.
Luke Marsden: But yeah, just say show me.
Luke Marsden: It's just going to take a little second there.
Luke Marsden: Oh, it hasn't even sent your message show me yet.
Luke Marsden: But it's already opened up the browser to check.
Tony: Can you click on that?
Tony: Can you make that bigger in there?
Luke Marsden: Yeah, you can, yeah.
Luke Marsden: And you can also go full screen if you look next to where it says L. Because that's me being in the session with you, Luke, at the top.
Luke Marsden: Up a bit.
Luke Marsden: Yeah, there's a little full screen button.
Luke Marsden: Oh, no, sorry, that's you, Leah.
Luke Marsden: I'm just used to my name being out, but yeah, you can go full screen to kind of like.
Luke Marsden: And now you're kind of in the agent's computer.
Luke Marsden: So it's, it's worked.
Leah Smith: Oh, nice.
Leah Smith: Amazing.
Luke Marsden: So, yeah.
Chris Sterry: So, Tony, just to give you some context, this is exactly what I'm doing when I'm doing prospecting in LinkedIn.
Chris Sterry: I'm having it go into LinkedIn under my user account.
Chris Sterry: It validates me through its own browser.
Chris Sterry: So I, you know, I have to do the two factor on my phone and it's doing exactly this and I've basically built out a plan of kind of what top prospects look like.
Chris Sterry: And I'm having it search following LinkedIn's rules for bots and things like that.
Chris Sterry: So it's looking and clicking in ways that a normal human would interact and things like that.
Chris Sterry: So I'm not getting errored out on it.
Chris Sterry: But then it will write messages if I want it to or I can approve everything and not have it do anything like.
Chris Sterry: So but for the prospecting or looking for maybe open job wrecks that you want to go reach out to and do something with.
Chris Sterry: Like it can do that through.
Chris Sterry: Through LinkedIn jobs as well.
Chris Sterry: And I use it all the time for prospecting.
Tony: Yeah, yeah, yeah.
Tony: Amazing.
Luke Marsden: Yeah.
Luke Marsden: And we can maybe do a dedicated session on that at some point.
Luke Marsden: Yeah.
Luke Marsden: Before just throwing.
Tony: But would I set up my own task in this to do that as well.
Luke Marsden: Yeah, yeah, yeah, you can.
Leah Smith: Okay.
Tony: This is the thing that I want it to do.
Tony: Would I have to send my kind of login credentials and stuff like that into the task?
Chris Sterry: So you would, you know, I. I don't do it in the task because I don't want it.
Chris Sterry: I only want it doing it when I'm available with it.
Chris Sterry: And then the other thing, you also don't commit it because it's kind of.
Chris Sterry: For me, it's always.
Chris Sterry: It's never going to finish in the full commit cycle.
Chris Sterry: But it two factors on your phone.
Chris Sterry: So I have to log in every time it prompts me to log in.
Luke Marsden: Yeah.
Chris Sterry: And then I two factor it on my phone and then it just works through.
Luke Marsden: And basically, Tony, you can copy and paste into the browser in the agent's desktop.
Luke Marsden: So you can just copy and paste the password from your password manager into the browser at the point at which it logs in.
Luke Marsden: You don't need to save the credential anywhere and.
Luke Marsden: And then it will just have a valid session for that link 10 login.
Luke Marsden: Leah, your screen seems to have frozen for me.
Luke Marsden: I don't know if it's.
Luke Marsden: Oh, no, it's come back now.
Luke Marsden: Okay.
Luke Marsden: It just needs to wiggle the mouse.
Luke Marsden: So that looks fine.
Luke Marsden: So now hit Open pr.
Leah Smith: Open pr, the green button.
Leah Smith: Oh, yeah, sorry.
Luke Marsden: Yeah, yeah, that's fine.
Luke Marsden: So that's going to tell the agent when you say Open pr, you're basically saying, I'm happy with that change.
Luke Marsden: And then if you click on the PR defined AI and that's going to open in GitHub and it looks like Tony's doing the work because it's his account.
Luke Marsden: And if you want to, you can click files change to see the code, but you don't need to because in fact, if you scroll down because what the.
Luke Marsden: What the agent does is it shows screen, it gives screenshots in the pull request to prove that it's done the right thing.
Luke Marsden: And then you can also from the pull request, go back to helix by clicking open in helix on that link there.
Tony: This is private.
Tony: This.
Luke Marsden: Yes.
Luke Marsden: Yeah, yeah, yeah.
Luke Marsden: That's fully private.
Tony: I'm gonna have to go, guys.
Luke Marsden: Yeah, I was just gonna say, Tony, you need to bounce.
Tony: Yeah.
Tony: If you send me the recording, I'll watch the end anyway, if.
Tony: That's awesome.
Tony: Yeah, of course.
Luke Marsden: Great.
Tony: Yeah, it's good.
Tony: Yeah.
Tony: Really excited.
Tony: Thanks.
Luke Marsden: Good stuff.
Tony: Yeah, yeah.
Tony: Next step.
Tony: Yeah.
Tony: If I can jump on and you can.
Tony: We can do a specific session on the kind of Agents for outreach.
Tony: That'd be amazing.
Luke Marsden: Yeah, yeah, yeah, that'd be really cool.
Luke Marsden: All right, Cheers.
Luke Marsden: Tony.
Luke Marsden: Stay cool if you can.
Leah Smith: I am.
Leah Smith: I might have to dash.
Leah Smith: I can hear my dad with Adrian.
Leah Smith: She has been crying for about.
Luke Marsden: Oh, bless.
Luke Marsden: Okay, you go and bounce, Leah.
Luke Marsden: That's the most important.
Luke Marsden: Go and do that.
Luke Marsden: Good luck.
Leah Smith: Yes.
Leah Smith: I'll start trying things in the background.
Luke Marsden: Yeah, if you just queue up a whole bunch of these tasks where you have things, or if you have content that you'd like to add to pages, just paste them into that text box and set them off.
Luke Marsden: And then just ping me on Slack with links inside the app and I can help you get stuff over the line.
Leah Smith: I'm just really conscious that I don't want to obviously rinse all the credits.
Leah Smith: I'll be checking it, like.
Luke Marsden: Don't worry, we can both keep an eye on those.
Luke Marsden: But you crack on.
Leah Smith: Um, yeah, so I have any.
Leah Smith: Any queries, I'll let you know.
Leah Smith: But, yeah, I think that made the Running the task seems, like, pretty straightforward and I've written down how to do it, but if I. Yeah, if I have any issues, I'll let you know.
Leah Smith: And I'll let you know when I've made some changes, so obviously.
Luke Marsden: Awesome.
Luke Marsden: Sounds good.
Leah Smith: Thank you so much.
Leah Smith: And what.
Leah Smith: When's next week's meeting?
Leah Smith: Sorry?
Luke Marsden: Because I think we're on Tuesday, if that's okay with you.
Leah Smith: I won't.
Leah Smith: I'm on holiday.
Leah Smith: Okay.
Luke Marsden: Oh, you're on holiday, because.
Luke Marsden: Cool, that's fine.
Luke Marsden: Good.
Leah Smith: If you could record the meeting and I could maybe watch it when I'm back, or I can join the week after and.
Luke Marsden: Yeah, yeah, yeah, that's fine.
Luke Marsden: Have an amazing holiday.
Leah Smith: Thank you so much.
Leah Smith: Thank you both.
Leah Smith: Thanks.
Leah Smith: Bye.
Tony: Cool.
Luke Marsden: I think that went all right.
Chris Sterry: Do you want to ping pren things and see if we all meet?
Luke Marsden: Oh, yeah, I'll just make a new zoom for it.
Chris Sterry: Perfect.
Luke Marsden: We can't see that.
Luke Marsden: Cheers.

