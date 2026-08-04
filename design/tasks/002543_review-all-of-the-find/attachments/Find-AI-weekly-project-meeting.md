# Find-AI weekly project meeting

- **Date:** Wed, 01 Jul 2026 13:00:00 UTC
- **Participants:** luke@helix.ml, chris@helix.ml, leah.smith@linuxrecruit.co.uk, tony.chapman@linuxrecruit.co.uk, luke@mlops.consulting
- **Source:** fireflies

## Summary

- **Website Enhancement:** AI-powered site lets non-tech users update content via chat, speeding edits with GitHub auto-deploys.

- **Backend Integration:** Dynamic content and job-candidate matching using anonymized profiles, enhancing self-service client sourcing.

- **AI Outreach Automation:** Agents mimic human LinkedIn activity, automating prospecting and messaging, cutting manual work by hours daily.

- **Team Coordination:** LinkedIn Recruiter, Bullhorn, and Slack sync prevent duplicate outreach and keep communications aligned.

- **Recruiter Productivity:** AI manages messaging, profile updates, and scheduling, freeing recruiters for higher-value client calls.

- **Controlled Automation:** Human review ensures AI messages keep personal tone and trust throughout gradual rollout process.

## Transcript

Luke Marsden: Hey, Chris.
Ethan: Yo.
Luke Marsden: Hey.
Chris Sterry: How's it going?
Luke Marsden: Good.
Luke Marsden: Yeah, doing great, thank you.
Ethan: How are you?
Chris Sterry: I'm good.
Luke Marsden: You're not knee deep in the river?
Chris Sterry: I'm not.
Chris Sterry: I was.
Chris Sterry: I was waist deep in the sea a minute ago.
Chris Sterry: But we got some lunch and.
Chris Sterry: And I want to jump on the call.
Luke Marsden: Amazing.
Luke Marsden: Well, Phil is cooking chicken downstairs as we speak.
Luke Marsden: Oh, I'm looking forward to that.
Chris Sterry: I had another good call today with.
Chris Sterry: Let me find him.
Luke Marsden: Let me find his info.
Luke Marsden: This guy.
Luke Marsden: Where is it.
Chris Sterry: Anyway?
Chris Sterry: Someone who.
Chris Sterry: He heard about us through a.
Chris Sterry: A Hannah meetup.
Luke Marsden: Oh.
Chris Sterry: But his wife is, so.
Chris Sterry: He has a small company, but they're in like.
Chris Sterry: I'm going to get a call with.
Chris Sterry: With the three of us here soon.
Chris Sterry: Yeah, hang on, let me pull it up.
Chris Sterry: Sorry, I'm trying to do too many things at once.
Chris Sterry: But his wife is a CFO at a company and she's like ghost using Claude for their stuff.
Luke Marsden: Interesting.
Chris Sterry: And he's like, I need her to not do that.
Chris Sterry: And you guys sound like the right kind of company to do it.
Luke Marsden: That's cool.
Luke Marsden: Awesome.
Chris Sterry: Yeah, so I'm using Helix for all.
Luke Marsden: Great to see how you're doing.
Luke Marsden: Hey, Tony, how you doing?
Tony: Yeah, not too bad.
Tony: Not too bad.
Tony: Yourself?
Luke Marsden: Good.
Luke Marsden: Yes, very well, thanks.
Luke Marsden: I'm staying with our co founder Phil at the moment up in York.
Luke Marsden: Okay, Nice, nice.
Tony: Whereabouts is that?
Luke Marsden: It's.
Luke Marsden: Yeah, near York.
Luke Marsden: Close to Selby.
Luke Marsden: Yeah.
Tony: Okay.
Luke Marsden: Nice.
Luke Marsden: Yeah, yeah, nice area.
Luke Marsden: It's lovely.
Luke Marsden: Yeah.
Tony: Yeah.
Tony: People are friendly, I guess.
Luke Marsden: Definitely.
Luke Marsden: I like the accent.
Luke Marsden: We went for fish and chips when I first arrived and I was like, whoa, I'm in a different place.
Tony: Everyone's saying hello to you as you're walking down the street up north, isn't it?
Luke Marsden: Yeah, exactly.
Luke Marsden: Good.
Luke Marsden: Yeah.
Luke Marsden: How's things?
Luke Marsden: How's Find AI Linux recruit world going?
Ethan: Yeah, yeah, good.
Chris Sterry: Yeah, Yeah.
Tony: I mean, we haven't really done much with.
Tony: Really.
Tony: It's kind of.
Tony: Yeah.
Tony: Here's Alex.
Luke Marsden: How are we?
Luke Marsden: Very well.
Luke Marsden: Nice to meet you.
Luke Marsden: I'm Luke.
Alex Case: Lovely to meet you all as well.
Luke Marsden: Cool.
Luke Marsden: Oh, you're both.
Luke Marsden: Yeah, yeah.
Luke Marsden: Great.
Tony: So, yeah, Alex is one of the associate directors here, I guess, very much the cold face, speaking to candidates daily, all that kind of stuff.
Tony: Speaking to clients.
Tony: I've given him a bit of an overview on the.
Tony: The kind of agents and stuff.
Tony: Like,.
Luke Marsden: Absolutely fine.
Luke Marsden: Because what I'd love to do is actually talk to you and work with you as we go through this to check, like, can we actually help?
Luke Marsden: Like, what?
Luke Marsden: What?
Luke Marsden: Or what?
Luke Marsden: Can we help what?
Luke Marsden: Would be most useful for us to build agents that do and.
Luke Marsden: And then iterate together.
Luke Marsden: That's kind of the approach I have normally.
Luke Marsden: Yeah.
Chris Sterry: And Alex, are you the skeptic?
Chris Sterry: I'm just curious where we land on this merge.
Alex Case: I think skeptic is perhaps a too strong a word maybe for how I feel.
Alex Case: I just, I know I haven't experienced what bad potentially this can do, to be honest.
Alex Case: So excited to see how it, you know, either supplements what I kind of do myself kind of manually or, you know, how I can help, really.
Luke Marsden: Okay, great.
Tony: I. I would add that Alex is probably the skeptic final boss, actually, who said I'm keen for him to see just how.
Tony: How amazing it is and just Alex, Nathan, you know, Chris and Chris and Luke have been great.
Tony: So.
Luke Marsden: Yeah,.
Chris Sterry: That's what he says in front of our faces.
Chris Sterry: But there's a real reason Alex is the skeptic.
Tony: No, but it's just been really good, you know, just, you know, talking through exactly what might help us and.
Luke Marsden: Yeah.
Tony: Worrying their minds, actually.
Tony: Yeah, we could do this.
Tony: We can do this.
Luke Marsden: So let me show you a few things then.
Luke Marsden: And Ethan, nice to meet you as well.
Luke Marsden: Cool.
Luke Marsden: Yeah, so let me show a few things.
Luke Marsden: I did prep a couple of things just to go through and then we just chat and brainstorm, I think.
Luke Marsden: Is that okay, Tony?
Tony: Sounds perfect.
Tony: Yeah.
Luke Marsden: Okay, cool.
Luke Marsden: So let me just pull up the right thing and I will share my screen.
Luke Marsden: So.
Luke Marsden: So yeah, I'll start with the website because that's the first thing that we've.
Luke Marsden: That we did and we're kind of one month into a three month project now.
Luke Marsden: Roughly keep me out of jail, Chris.
Luke Marsden: I think that's about right.
Luke Marsden: And so what we have so far is we put together.
Luke Marsden: Oh, no demo.
Luke Marsden: Give me one minute.
Luke Marsden: Sorry.
Ethan: Not a good start.
Chris Sterry: We're not.
Chris Sterry: We're not helping Alex here.
Chris Sterry: This is.
Tony: I don't know how obviously on this.
Alex Case: Call for four minutes and somehow I'm.
Tony: The bad guy already.
Chris Sterry: Yeah, he.
Chris Sterry: There was an email that went out before and said you're the bad guy and lots of things.
Chris Sterry: So, yeah, we're just, we're just filling the gaps and I apologize if I look like I just came in from the beach.
Chris Sterry: I did.
Chris Sterry: So, I mean, I don't apologize, but I'm.
Chris Sterry: I'm vacationing in Croatia right now, so just.
Luke Marsden: Wow.
Chris Sterry: Just came back.
Chris Sterry: I'm leaving for Italy tomorrow, but just came back for the family to get on with the family to get on this call and have some lunch and so if I Look disheveled.
Chris Sterry: I, I actually am disheveled.
Chris Sterry: So it works out.
Luke Marsden: Chris joined the call the other day and he said, I'm neat, I'm waist deep in a river to taking this call.
Luke Marsden: And I'm like, Chris, you really can just have your vacation if you want to.
Luke Marsden: And he's like, no, no, no, I want this.
Chris Sterry: I, I'm.
Luke Marsden: I'm.
Luke Marsden: This is my best life.
Ethan: Amazing.
Chris Sterry: I was literally waist deep in a river as well.
Chris Sterry: I had my AirPods in and holding my phone out and just talking.
Chris Sterry: It was great.
Tony: Nice.
Luke Marsden: Let me.
Luke Marsden: Okay.
Luke Marsden: I can do it like this.
Ethan: Okay.
Luke Marsden: So.
Luke Marsden: So yeah, basically if we start with the website.
Luke Marsden: So this is a new website we put together for the Find AI brand.
Luke Marsden: The.
Luke Marsden: Of course it's pretty basic at the moment, but we are going to be able to have you guys or Leah or Tony kind of update this by using an agent to modify it.
Luke Marsden: For example, the first thing that Leah was able to do was add this trusted by banner.
Luke Marsden: And she did that just by chatting to an agent.
Luke Marsden: In fact, I can probably show you, I think it was this merged change.
Luke Marsden: And should I zoom in or is this.
Luke Marsden: Zoom in a little bit.
Luke Marsden: Okay.
Luke Marsden: Yes.
Luke Marsden: Yeah.
Luke Marsden: So Leo was able to put together this.
Luke Marsden: Use company logos for client carousel.
Luke Marsden: And what you can see here is basically she just like had a chat with the agent and was able to pull in the screenshots and it created a change to the website that she was then able to merge.
Luke Marsden: So what this is basically doing is it's giving Leah a web developer that she can talk to and the web developer is an agent.
Luke Marsden: And so I put together a.
Luke Marsden: Well, I put together a silly example earlier.
Luke Marsden: I kicked off this one.
Luke Marsden: We're not going to merge it because it looks awful.
Luke Marsden: But I just said all I want you to do is make the homepage bright green.
Luke Marsden: And it did.
Luke Marsden: So it's just a simple example of how you don't need to be able to write HTML and CSS anymore in order to put these things together.
Luke Marsden: And then if you wanted to, you could open the pull request and merge it on GitHub and then it would automatically deploy.
Luke Marsden: So, yeah, that's kind of phase.
Luke Marsden: One of the plan that Tony and I talked about was, was get the website up and running and, and then integrate it with your database in the background, which, which we're going to do next.
Luke Marsden: And.
Luke Marsden: And so on the.
Luke Marsden: The next phase though is to Tony.
Luke Marsden: Have you told everyone about Jack and Jill?
Tony: A little bit.
Tony: I'll kind of mention it briefly, but not.
Tony: No, it'd be good.
Tony: It'd be good to just give the guys.
Luke Marsden: Yeah.
Luke Marsden: Do you want to describe it from your, in your words?
Tony: Yeah, we, we all know Jack and Jill and what they're trying to kind of create.
Tony: So we're only calling it a Mini Jack and Jill because that was the easy way for us to describe it to Luke.
Tony: But instead of just having a job search function on the website, we're going to have a candidate kind of profile creation facility on there.
Tony: So anyone you speak to from Bullhorn, as soon as you put them on Bullhorn, for instance, they can become searchable if they want to, or if we want them to become searchable, but it'll be a mini anonymized profile and a client can go on essentially search candidates on the Mini Jack and Jill and get in touch with us if they want to speak to them.
Tony: So it just enables clients to do their own kind of semi sourcing on candidates that we, we've kind of pre qualified, I guess, doesn't cut us out.
Tony: So we're not like Jack and Jill is almost like you get in touch with the person directly and you have to manage them and that kind of thing.
Tony: This is just a client alert service.
Tony: They're like, look at that person.
Tony: Okay, cool, let's, let's speak to them and see if we can get them, you know, chatting together as, you know, a normal agency fee kind of thing.
Tony: So yeah, we're kind of calling it Mini Jack and Jill, but essentially for candidates, it's going to be more of an AI job search functionality on there and from a client's perspective, they can upload a job description or upload an advert or whatever and it will push potential candidates to them.
Tony: So I see it as also like a bit of a BD tool where companies might go on there, upload their details looking for this kind of person, push them some, some results.
Tony: Okay, cool.
Tony: Get in touch with us to, to speak to them kind of thing.
Tony: So, so yeah, we're just trying to integrate some kind of AI stuff and create some kind of sourcing functions that clients can do themselves and some job searching functionality for candidates, basically.
Tony: So, yeah, Mini Jack and Jill we're calling it, but don't say that to anyone else who says we don't get sued by Jack and Jill.
Tony: So we changed the name, didn't we,.
Luke Marsden: To Rosie and Jim.
Tony: Rosie and Jim.
Luke Marsden: We can do bunch of duty if you prefer.
Ethan: Cool.
Luke Marsden: So.
Luke Marsden: So yeah, and I think it's actually worth talking through a little bit, kind of.
Luke Marsden: So that Was one piece is.
Luke Marsden: Is kind of enhancing the website with kind of features that make sense for candidates to more easily be able to search the database, maybe with a conversational view and then also making it easier.
Luke Marsden: Well, actually, Tony, how do you see it working from the, from the client's perspective in terms of the.
Luke Marsden: Because one of the agents is for, like candidates to match with jobs and the other one is how would it help the clients themselves and Ethan's job easier, helpfully?
Tony: Yeah, well, no, I think, I think for the guys, they're going to be more interested in how agents can help them source candidates for them.
Tony: So I think we talked about agents kind of logging into your LinkedIn in the desktop or in the kind of AI desktop and essentially behaving as they would, you know, Alex.
Tony: In males.
Tony: And, you know, they get up in the morning and I've got 20 messages from great people that they can just jump on a call with.
Tony: So it kind of frees them up to have on the phone rather than doing this, the searching piece, if that makes sense.
Chris Sterry: Chris, do you want me to show my workflow?
Luke Marsden: I mean, I was going to show this, but then if you want to show yours as well, then by all means.
Luke Marsden: So, yeah, so this, basically, we're already using the idea with all of this with Helix, with agents in general, is how can you reduce toil, like tedious work that is really boring, and then focus on the stuff where it's more interesting to do or you can do more of it.
Luke Marsden: And so, for example, right here, I've asked Helix to log me into LinkedIn and then find some potential clients.
Luke Marsden: So I'm actually going to log into my LinkedIn.
Luke Marsden: And by the way, this frame here is the agent's own computer.
Luke Marsden: So if you close your laptop lid and walk out of a cafe or whatever, it will carry on working in the background.
Luke Marsden: It can run overnight, it can carry on going.
Luke Marsden: And you've got a real browser with a real desktop environment here.
Luke Marsden: What I'm going to do is I'm actually going to log into my LinkedIn and copy paste directly into here.
Luke Marsden: You don't need to give your password to our system at all.
Luke Marsden: It just goes straight into, into there.
Luke Marsden: And then on my phone, it pops up with a LinkedIn two factor auth request.
Luke Marsden: Just say, yes, it's me.
Luke Marsden: I really am trying to log in here.
Luke Marsden: And then it logs me in in the browser and then I can say like, I'm in.
Luke Marsden: Carry on.
Luke Marsden: And the initial instruction I gave it, you can even see Brian Cantrell in reverse real time being rendered to me through the video stream.
Luke Marsden: And, and what I asked it, I said open LinkedIn and let me log in.
Luke Marsden: So it, and then it's, it's actually researched my head of search, head of AI hiring in, in LinkedIn and it's, it's going to go and use my network to find you some potential customers.
Luke Marsden: Some potential clients.
Tony: Did you prompt it to search posts or has it done that itself?
Luke Marsden: No, it's just I think last time it, it tried something else.
Luke Marsden: But yeah, it's, I didn't prompt it but you could if you wanted to.
Ethan: You.
Luke Marsden: And you can.
Luke Marsden: That it's doing it.
Luke Marsden: You can say oh no, go and look at, go and look at something else.
Luke Marsden: But yeah, I'll just pause there.
Luke Marsden: What, what, what do people think about this?
Luke Marsden: Does this seem like it might reduce toil or do you have worries?
Alex Case: I guess what, what would happen if next in terms of it, it's finding opportunities.
Alex Case: How does that then?
Luke Marsden: Yeah, yeah,.
Chris Sterry: I do this literally.
Luke Marsden: Oh, go ahead, Chris.
Chris Sterry: Can you hear me?
Chris Sterry: Sorry, I don't know if my answer is.
Luke Marsden: I can hear you loud and clear.
Luke Marsden: Yeah, yeah.
Luke Marsden: Oh.
Chris Sterry: So I use this every day.
Chris Sterry: So I have a prompt that I start and I can show you what I do that I call my daily workflow and I prompt it and I keep adding to it that I call like I literally say start my daily workflow.
Chris Sterry: I can show you what it looks like.
Chris Sterry: It's going out and looking for like opportunities for Helix.
Chris Sterry: So companies that are posting.
Chris Sterry: It's reading posts on LinkedIn for me, it's looking for connections, it's looking at my connect faces, it's looking at who I've chat with and the last time I.
Chris Sterry: Account.
Chris Sterry: Suggest and not have it do anything as we've gotten, I say we.
Chris Sterry: That's probably, I don't know what the right vernacular is at this point, but as, as I've worked with it day in and day out, I'm allowing it to respond for me in a lot of ways.
Chris Sterry: Not every way.
Chris Sterry: There's certain things I don't want it doing and I still say, hey, I want you to suggest a response and we'll move forward and things like that.
Chris Sterry: But I mean I can show you exactly how I use it and how I use it multiple times a day.
Chris Sterry: It's also writing posts for me now.
Chris Sterry: It wrote a LinkedIn post for me earlier today.
Chris Sterry: I fed it a lot of the language I use through Slack, through email, through a lot of things.
Chris Sterry: So it really has my tone in Mind as well, like the way I speak and all that stuff.
Chris Sterry: And part of my big goal is you want to eliminate as much AI slop as you can.
Chris Sterry: I'm not going to say it's never going to have AI slop in it, but I think I've done a really good job of over time understanding what I want it to do and I continue to build it to be able to use it as a tool that does the grunt work, the dirty work, the stuff that I need to do in going out and finding people, posting and finding people saying things or finding companies that I don't know about.
Chris Sterry: I have 30 plate.
Chris Sterry: I really want invest the time to do those things.
Chris Sterry: It's going to take me two, three hours.
Chris Sterry: This just does it.
Chris Sterry: And then the way I had it initially set up was to say tell me when you find something and then let me go figure it out.
Chris Sterry: But now it's doing some of that work as well.
Ethan: Yeah.
Luke Marsden: And so, so Chris, I'll show a couple of things and then.
Luke Marsden: No, we can hear you.
Luke Marsden: It's fine.
Luke Marsden: So I'll show, I'll show a couple of things and then if Chris's network is holding up, I can, we can pass it back back to you.
Luke Marsden: So this is the next piece of the puzzle, Tony, which is about the.
Luke Marsden: I know we talked about the Jack and Jill piece but, but beyond that, the kind of the third part of the project is exactly what we were just talking about.
Luke Marsden: Alex, Ethan and the rest of your team like helping you do less toil.
Luke Marsden: So it's like sort of stuff that will actually assist you and you'll agent's own the browser.
Luke Marsden: So you asked like, so if, if we, if we've got some leads from this LinkedIn agent or LinkedIn bot, where do they go and, and how do we manage that?
Luke Marsden: So the, the first piece here is we're putting together, I'm calling it Finder and it's a bit of a funny name for it, like operating system.
Luke Marsden: It's a bit of a nerdy name but there's this sort of, the industry seems to be converging on kind of company OS.ie will give you an operating system, like a runtime environment for your agents that is like a dashboard for you to help, like see what it's doing and drive different things.
Luke Marsden: So literally just putting this together now and this, like I said, it's the start of this section of the project.
Luke Marsden: So this is like a sketch really.
Luke Marsden: But, but we're still filling out the details.
Luke Marsden: You can, you can dispatch different Tasks So you can say I want you to do things.
Luke Marsden: Get this kind of list of prospects.
Luke Marsden: So you might have.
Luke Marsden: This is just from a previous put together but every time the agent finds a potential prospect.
Luke Marsden: Or it might be match for a potential job and this will integrate with your CRM.
Luke Marsden: Remind me what it's called Tony.
Luke Marsden: CRM.
Luke Marsden: The.
Luke Marsden: Yeah yeah, yeah.
Luke Marsden: So we're integrating with.
Luke Marsden: With Bullhorn.
Luke Marsden: But you could just imagine that this says like potential customers and potential prospects.
Luke Marsden: Kind of people that have that it's discovered using.
Luke Marsden: Using your LinkedIn or using Reddit or GitHub or whatever and, and, and yeah.
Luke Marsden: So you.
Luke Marsden: And then there's also an option for outreach.
Luke Marsden: You can have it go and, and kick off a sort of social pulse.
Luke Marsden: Like maybe you want to dig into some specific area that you're recruiting for.
Luke Marsden: You can put in like search terms in here and you can go.
Luke Marsden: You can.
Luke Marsden: So you can say like go and sweep LinkedIn, go and sweep X and then pull my discovered prospects in here.
Luke Marsden: Synchronize them with the database.
Luke Marsden: And the other thing is that we also integrating with Slack so you can have a conversation with your agents while they're running and doing this work just through Slack so you can help steer them without even having to come in here.
Luke Marsden: And then the Slack bot will give you.
Luke Marsden: Or the agent in Slack will give you links back in here to come and look at things when it's found new things.
Luke Marsden: So different actions, different pipeline.
Luke Marsden: We've also got integrations here with things like Fireflies.
Luke Marsden: I think you use a different one for meeting transcript.
Tony: Sorry device it's called.
Luke Marsden: Yeah yeah.
Luke Marsden: So we'll do an integration with that so that you can then fetch transcripts from meetings and then get.
Luke Marsden: Get it to start doing things off the back of that.
Luke Marsden: And you can put different messaging in here like this is the Helix messaging but you can put whatever messaging you want to and get it to kind of lean.
Luke Marsden: Lean off that.
Tony: Of their time is spent looking for candidates.
Tony: So rather than it's normally kind of myself he's going out looking for.
Luke Marsden: I remember you saying yeah, yeah, yeah, yeah.
Luke Marsden: You do the clients and they do the candidates.
Tony: Yeah the business side that would be all that kind of workflow would be amazing for just for outreach for that for the candidate side.
Tony: I think the, the beauty for.
Tony: For YouTube and obviously the rest of the team is you're going to want to feed all the.
Tony: Into an agent.
Tony: That agent will then source for you and send some outreach to send some messages to people that hit a certain bar or match the criteria or whatever, you'll be able to dictate what that message says.
Tony: Like Chris was saying, you can manage the responses yourself initially, but hopefully we can get to a point where the AI agent is just managing them as if they're you because obviously they've seen the way you're responding to messages up to the point where it's here's my cv, jump on a call or whatever.
Ethan: Yeah.
Tony: And also log into Bullhorn and have the agent, as soon as that happens, the agent is then putting that profile onto Bullhorn for you automatically.
Tony: So all you've got, what it's doing is doing the entire outreach for you.
Tony: Obviously you're still going to be doing that as well, but it's going to be doing a lot of that behind the scenes and you're just going to be given profiles on Bullhorn of people who are really good candidates and open to a conversation.
Tony: And even not, you can set up like a calendar link kind of thing and the AI agent can just book calls into your calendar kind of thing.
Tony: So you just get up in the morning and you've got calls scheduled for good people.
Tony: Is the.
Tony: It's kind of what we want to get.
Luke Marsden: Yeah, yeah.
Tony: So it's all possible essentially, you know, it's.
Tony: If we're feeding it in the right info, it's all possible, isn't it Luke?
Luke Marsden: Absolutely.
Luke Marsden: And yeah, I'd love to get some feedback from Alex and Ethan on that.
Luke Marsden: And how does, how does that sound?
Luke Marsden: Would you like to.
Luke Marsden: Are there other time consuming tasks you'd like to consider?
Luke Marsden: Trying to chop out that kind of thing?
Ethan: I think for my time.
Ethan: I'm quite intrigued to know how it in like with LinkedIn would that not be like.
Ethan: If it's messaging people are like, I don't know, 10, 10pm a night or in the morning, etc, does that not flag on their system or does it go.
Chris Sterry: Yeah, so great question.
Chris Sterry: So a couple things when I built this one, I had it study LinkedIn's bot detection.
Chris Sterry: Everything published about LinkedIn's bot detection and how to avoid that because we're in a browser in your account two factor auth all that stuff.
Chris Sterry: We've omitted a bunch of that bot detection piece already.
Chris Sterry: But there's still, if it behaves like a bot instead of a human, it's going to be detected.
Chris Sterry: So I have it when it's looking at things, scrolling, clicking, moving in different pages, looking at my own profile.
Chris Sterry: That's one of the factors that LinkedIn looks at it at kind of bot.
Chris Sterry: If you Your arrogance level I guess we'll call it things like that, you know, and figuring that stuff out.
Chris Sterry: The other thing is yeah you, it's for me I can only send really about 15 messages a day as a cold outreach message.
Chris Sterry: That doesn't mean though it stops there.
Chris Sterry: So a net new I make a connection, I send a message, I do that stuff.
Chris Sterry: That's about 15 a day and that's part of my daily workflow.
Chris Sterry: But I have it scrolling for posts and searching different things, searching companies job recs and then going and looking at that company's website and figuring out who is most likely to go post that job wreck based on title and then reaching out to them, finding their email, doing multiple things than just LinkedIn.
Chris Sterry: So using LinkedIn and I apologize I turn off my video because I think my, I was a little sketchy on my, my audio and the Internet here but so I have it taking it a next step and saying okay so great LinkedIn jobs is a great way to find people who are hiring for what Helix offers.
Chris Sterry: Let's go look at that company more and let's go see who's there and let's go outreach in a different fashion that way and then if I'm already connected to them that doesn't factor into the LinkedIn count so that won't negatively affect me.
Chris Sterry: But also if they're posting on LinkedIn, not just the job thing but there's another post sometime and I have it at a two week window where I don't want to see anything that hasn't that's, that's not two weeks newer or less because I think everything else is kind of slop.
Chris Sterry: If it responds to something that was posted months ago, it can reply to that posting as well and then I can reach out and I can, once we start communicating that way I can that that's another way to kind of be able to use LinkedIn but not fall within their, their boundaries of what I can just say to people like as spam.
Chris Sterry: And that's literally every day it runs.
Tony: And we spoke about stuff like you know, searching GitHub for, for canvas and that that's all you know that can all just work behind the scenes.
Tony: You know, you set up and it just does it for you.
Tony: So we've always struggled to get around how do we actually search and contact people via GitHub.
Tony: You know we've had a few, few wins there but this can just look at profiles while you're doing other stuff and you might spin up a few people and A few conversations from that because it's.
Tony: It's a link to your email as well.
Tony: You can set this loose on doing that essentially.
Luke Marsden: Yeah, you can think of it like giving you both your own minion and it's a sort of nerd minion that can go and do stuff for you.
Ethan: So,.
Luke Marsden: For example, I've had success if I need to.
Luke Marsden: If I need to connect with someone and I only know like the GitHub handle, you can get it to go and clone one of their repos and then look through the commits and find the email address in the commit message or in the commit.
Luke Marsden: And then you can get it to like, oh, and now like, put those back into the database.
Luke Marsden: So I've now.
Luke Marsden: And then ping me on LinkedIn when you found 10 of them, or something like this.
Luke Marsden: Sorry, ping me on Slack when you found 10 of them.
Luke Marsden: It should be pretty flexible.
Luke Marsden: And as we work through this together, we're going to start building out the framework for doing that, the connections with your Slack, the dashboards you want to see which we can change very easily and hopefully just start getting you using it and giving us feedback and then make it actually useful for you.
Luke Marsden: Because I understand the skepticism.
Luke Marsden: Like, a few years ago this wasn't possible and it was pie in the sky.
Luke Marsden: But we use it all the time ourselves now, both for software development and Chris's work as well on outreach.
Chris Sterry: And I would love to just work with you on like, hey, let's set up a sample project, right?
Chris Sterry: So really lightweight.
Chris Sterry: Just here's something I got to do every day that's 30 minutes to an hour that if I could not do that every day in the same way I do it and make it 10 minutes or, you know, weekly or whatever that is.
Chris Sterry: Right.
Chris Sterry: Let's take a small chunk of like, here's something I do on a regular cadence and give you that time back and just show how to not fully automate it right now, but get it to a point where now it's just handing you the information and saying, what do you want to do with it?
Chris Sterry: Here's my suggestions.
Chris Sterry: And then moving forward there, because I want to be very wary of, like, you have your voice, you have your way of working, you have your.
Chris Sterry: The way you interact, that makes you successful.
Chris Sterry: So I don't want to just put that on the, on the back burner.
Chris Sterry: I want to make it to a point where it starts to understand you versus you having to kind of work within its boundaries.
Chris Sterry: And I think that's the best way to kind of start Operating with things like this.
Luke Marsden: Yeah.
Luke Marsden: Cool.
Luke Marsden: I want to make some more space for Alex Nathan to share thoughts.
Alex Case: I think that sounds pretty good from my perspective as well.
Alex Case: But the thing I'm kind of thinking about is if Ethan, myself and obviously the rest of the team all have our own versions of this kind of running on in the background.
Alex Case: Is there any kind of, I guess, kind of cross communication whereby if we are doing similar searches or prompting it to do similar things, it's not reaching out to the same person on LinkedIn, you know, four or five times because we're telling it to do the same thing.
Alex Case: Right.
Alex Case: Or conversely, someone maybe that Ethan spoke to last week, my agent, is now kind of reaching out with the, the same opportunity or something.
Luke Marsden: How do you normally avoid doing that?
Luke Marsden: Is it through.
Luke Marsden: Through the database?
Alex Case: Yeah, yeah.
Alex Case: So that would be through.
Alex Case: Through Bullhorn.
Alex Case: Yeah, exactly.
Alex Case: Or, or I guess just, you know, from, from talking about specific people in Slack and things like that.
Luke Marsden: Yeah, yeah, yeah, yeah.
Tony: On LinkedIn recruiter, Luke, it will, if you go onto a page, it will say so and so is in mailed this person on this, this date kind of.
Tony: You see the message.
Luke Marsden: Yeah, I mean, and we'll be, we'll be using LinkedIn Recruiter through the agent.
Luke Marsden: So if LinkedIn Recruiter can already tell you that on your team, then then A will do that.
Luke Marsden: B, we're going to integrate it with Bullhorn so that everything goes through Bullhorn and you're not like caught out.
Luke Marsden: The agent will have visibility into who else is coordinated with, with that person or communicated with that person.
Luke Marsden: And, and yeah, the, the agent can also.
Luke Marsden: The agents can also pick up like context from Slack because they're plugged into the Slack channel.
Luke Marsden: They get all the Slack messages.
Luke Marsden: So they will have a sense of that.
Luke Marsden: And, and they can even then join in with, with the chat.
Luke Marsden: But yeah, like I say, think of it as like personal minions coordinated and the.
Luke Marsden: Yeah, go ahead.
Chris Sterry: But I would say the third piece of that, Alex, is at first we don't let it run wild, right?
Chris Sterry: It's.
Chris Sterry: I want to verify the things so as we start seeing them, I want to make sure it's connected well.
Chris Sterry: I want to make sure when we're writing this markdown file as memory of what it should be addressing and how it should be looking at data that it doesn't miss a step.
Chris Sterry: So when I start, I wanted to suggest do you actually X, Y and Z and maybe, you know, it might take you a couple minutes longer to just cross reference that database to ensure.
Chris Sterry: But I would say, you give it a week and once you have that assurances that it's no longer doing the thing, because the first couple times it might.
Chris Sterry: But once you have that assurance, then you understand, oh, this is working well now.
Chris Sterry: And so that's why I just always say, like, we start with the human in the loop and you knowing what the heck's going on.
Chris Sterry: Because if not, that's where things get a little crazy when you're not sure and you just say, go.
Luke Marsden: Yeah, yeah.
Luke Marsden: And that's the upside of the agent has its own computer that you can screen share with it, basically, because you can literally see what it's doing.
Luke Marsden: And so when I started using it for LinkedIn outreach, I would insist on writing all the messages myself because I didn't want to come across as an AI.
Luke Marsden: But then it starts suggesting messages that are basically like what I just said, but with specific detail for the other person.
Luke Marsden: I'm like, okay, carry on.
Luke Marsden: I kind of trust you now.
Luke Marsden: And you're not, you're not, you're not.
Luke Marsden: You don't sound like Claude.
Luke Marsden: Right?
Luke Marsden: Because that's the last thing we want.
Luke Marsden: We don't want to sound like Claude.
Luke Marsden: I'm constantly like, no, rewrite this in my.
Luke Marsden: In my voice.
Luke Marsden: But, yeah, gotcha.
Alex Case: Okay.
Alex Case: No, that sounds good.
Tony: Oh, sorry, go on.
Ethan: That's right.
Ethan: Will it just be looking at LinkedIn recruiting?
Ethan: Because what I've seen is that not every engineer from like, let's say Legora, for instance, is one of the companies that we could target for AI.
Ethan: AI people.
Ethan: On LinkedIn Recruiter, it comes up with just certain people, whereas the actual LinkedIn it can come up with different people.
Ethan: So does it just do LinkedIn recruiter and people that are maybe a bit more active, whether they've come, responded to emails from other people, etc.
Ethan: They usually come up.
Ethan: Or does it do the whole LinkedIn kind of database?
Luke Marsden: We can do both.
Luke Marsden: Yeah.
Luke Marsden: Yeah.
Luke Marsden: We can basically just point it wherever is working and then you can also have a discussion with it to be like, I'll try this, or like.
Luke Marsden: And it will also.
Luke Marsden: It can also ping you on Slack if it needs help.
Luke Marsden: So if it gets stuck to something, it can ask, it can ask you.
Luke Marsden: So, yeah, if LinkedIn is.
Luke Marsden: And we can sort of write down in English what we want it to do.
Luke Marsden: And I'll show you that on another call in the future where you can sort of.
Luke Marsden: You basically say, like, you can find candidates on LinkedIn.
Luke Marsden: If you do, you have to cross reference them with Bullhorn to make sure no one else is talking to them.
Luke Marsden: If you're using LinkedIn Recruiter, it will already tell you whether someone else is talking to them, that kind of thing.
Luke Marsden: So whatever rules we need to give it, we can figure those out together.
Tony: I would double check filters or anything because then LinkedIn Recruiter is literally a replication of LinkedIn, so there shouldn't be anyone on one or not on the other.
Tony: So.
Ethan: Yeah, well, I feel like, sometimes I feel like I can search.
Ethan: If I search on just the usual search bar on LinkedIn, it can come up with different or more people that I haven't seen on.
Ethan: On like a actual search on Recruiter.
Ethan: I don't know if that's just me or if it's maybe my search, but like I didn't come up with like everyone in Ligora.
Ethan: If I put ligor in a LinkedIn recruiter, it doesn't come up with everyone in there.
Ethan: In there.
Luke Marsden: But that's just.
Tony: Yeah, just one quick use case.
Tony: So I'll just, just thought obviously to.
Tony: I think I've mentioned this to you as well, like one way that we pick up new businesses.
Tony: We'll find out, you know, where market intel from candidates, you know, they're interviewing at this place or whatever.
Tony: If we find out, there's a process we follow to kind of follow that up, as in find out who the, you know, the managers are there and then find out their email addresses and send them a stock email.
Tony: So if we've got a stock email with, you know, a couple of PDFs ready to go, we can just tell the agent every time we get a lead, we can just chuck the name Tesco or whatever it might be into that, into that particular workflow and then the agent will go through the whole process of following that up for us instantly.
Tony: But Chris.
Tony: Yeah.
Tony: You mentioned about working with us to set up a couple of initial workflows.
Tony: I think perhaps if I do a couple with you, one, one candidate side for a specific skill set and one potentially new business side and we can iterate on that a little bit and then take that to Alex and Ethan.
Luke Marsden: And say this is what we've come.
Tony: Up with and this is what, how it can help you and, you know, we can see what the pain points might be initially, I'm more than happy to jump on whenever, whenever you're available to do that.
Tony: That's cool.
Chris Sterry: Yeah, I'd love to.
Chris Sterry: I have so much fun.
Chris Sterry: Like, honestly, like, I can't.
Chris Sterry: I know it's my product, so I'm a bad source.
Chris Sterry: Of information, but it makes.
Chris Sterry: I'm very biased in it, but it makes my life so much easier in the stuff that just takes me so much time and like junk I got to sift through to get there.
Chris Sterry: And instead I literally just have a prompt that says start my daily workflow and then it prompts me.
Chris Sterry: Okay, we're going to do this.
Chris Sterry: Here's what I found here.
Chris Sterry: Go do this.
Chris Sterry: And what would take me.
Chris Sterry: I used to call it LinkedIn doom scrolling.
Chris Sterry: I would just doom scroll for hours and looking for certain things and it's doing all that doom scrolling for me and just giving me the result and it just is so much nicer for me because I get to do the stuff I want to do instead of just LinkedIn doom scrolling now.
Tony: Yeah, yeah, sounds ideal.
Luke Marsden: Cool.
Luke Marsden: Any other questions or any other topics?
Alex Case: I don't think for me.
Alex Case: Not this time, anyway.
Luke Marsden: Cool.
Luke Marsden: Well, let's see.
Luke Marsden: This is the start of a collaboration.
Luke Marsden: That's how I'd like to think about it.
Luke Marsden: It'd be lovely to get you.
Luke Marsden: You guys like looped into some.
Luke Marsden: Some future meetings once we've actually started building a bit more of this out.
Alex Case: Yeah, for sure.
Luke Marsden: And yeah, it's only going to be successful if we actually iterate together.
Luke Marsden: So that's how I'd like to approach it.
Luke Marsden: Cool, Nice.
Alex Case: That sounds good to me.
Alex Case: Thank you for the explanation.
Luke Marsden: Yeah, thanks both.
Luke Marsden: Anything else, Tony, from your side?
Tony: No, I don't think so.
Tony: Yeah, I was just keen to explain what it could do to Alex and Ethan.
Alex Case: Really?
Chris Sterry: Yeah.
Chris Sterry: When do you want to set up that time?
Chris Sterry: I mean, let's.
Chris Sterry: Let's make it as soon as you want.
Tony: Yeah, I mean, I'm free whenever, really.
Tony: Tomorrow?
Tony: Tomorrow's good.
Tony: Thursday.
Tony: Friday is good as well.
Tony: I'm conscious you're away though.
Tony: When's work best for you?
Chris Sterry: Oh, good question.
Chris Sterry: I just thought about.
Chris Sterry: I'm traveling tomorrow.
Chris Sterry: I think Friday works.
Chris Sterry: I just don't quite know how good the WI fi situation is.
Chris Sterry: I'm on a weird place in the middle of nowhere.
Chris Sterry: So let me figure that out tomorrow and then I'll just.
Chris Sterry: I'll shoot you a note.
Chris Sterry: But Luke can also help with this too, so.
Chris Sterry: And he has.
Chris Sterry: We got weekly meetings and all that stuff too, so.
Ethan: Yeah.
Tony: Yeah, I can have a play around myself, can't I?
Luke Marsden: Yeah, yeah, yeah.
Chris Sterry: Have you been in it yet?
Tony: Maybe.
Tony: I don't actually.
Tony: I've got the website.
Tony: The website task thing of an ey.
Tony: Is that in the same place or is it.
Luke Marsden: Yeah.
Luke Marsden: What I'd like to do is I'd like to keep cranking a little bit on.
Luke Marsden: On the stuff that we're building, kind of a little bit more structure around it.
Luke Marsden: And so maybe next week we could jump on and.
Luke Marsden: And do that and.
Luke Marsden: Yeah, Chris, I'll sync up with you on.
Luke Marsden: On that as well.
Luke Marsden: And if you've got time next week, I know Chris is going to be in Bristol with me in a few weeks as well.
Luke Marsden: So we'll.
Luke Marsden: We'll have time then.
Luke Marsden: Yeah, Amazing.
Tony: Let me know next week.
Tony: Just let me know.
Tony: Timing on.
Luke Marsden: Brilliant.
Luke Marsden: All right, awesome.
Luke Marsden: All right, Perfect.
Chris Sterry: Thank you very much.
Chris Sterry: Thanks, everyone.
Luke Marsden: Nice to meet you, Alex.
Tony: Likewise.
Luke Marsden: Take care, Tony.
Ethan: Bye.

