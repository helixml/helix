# Find-AI weekly project meeting

- **Date:** Mon, 27 Jul 2026 14:00:00 UTC
- **Duration:** 58 min
- **Participants:** luke@helix.ml, chris@helix.ml, leah.smith@linuxrecruit.co.uk, tony.chapman@linuxrecruit.co.uk, luke@mlops.consulting
- **Source:** fireflies

## Summary

- **LinkedIn Automation:** Use semi-automated evening runs with human oversight to reduce ban risk, avoiding simultaneous logins on multiple devices.  
- **Product Development:** Multi-user job hierarchy supports several agents per job; file uploads and audio transcription enhance sourcing context.  
- **Operational Workflow:** Projects are shared among recruiters; manual review filters candidates to focus outreach and maintain quality.  
- **Platform Stability:** Addressed website outage promptly; agent is stable though slow, with filtering reducing workload and improving efficiency.  
- **Business Priorities:** Target Mini Jack and Jill brand launch by early September; billing moved to credit card for $499/month subscription.  
- **Team Coordination:** Evening agent runs require logged-out LinkedIn sessions on other devices; UI improvements planned for simpler recruiter use.

## Transcript

Luke Marsden: Hello.
Leah Smith: Hello.
Leah Smith: How are you?
Luke Marsden: Yeah, I'm good, thank you.
Tony: How are you?
Leah Smith: Yeah, good, thank you.
Leah Smith: How was your weekend?
Luke Marsden: It was lovely, thank you.
Luke Marsden: Yeah, I had my co founder Chris here all of last week.
Leah Smith: Oh yes, I remember you saying.
Leah Smith: Did you have fun with him?
Luke Marsden: Yeah, no, it was great fun.
Luke Marsden: It was a good mix of working and introducing him to my friends because he introduced me to his friends when I went out to Oregon, so we all had a good time.
Leah Smith: Is he left now?
Luke Marsden: Yes, I think he, he, he got the coach from Bristol coach station at like 3am yesterday morning and I think he arrived home a few hours ago.
Leah Smith: And where is.
Leah Smith: Was.
Leah Smith: Is it the U.S. isn't it?
Luke Marsden: Yeah, he's.
Luke Marsden: Oregon.
Leah Smith: Oh yeah, Oregon.
Luke Marsden: Yeah, he's in Bend, Oregon.
Luke Marsden: Last I heard from him was like, oh, my flight from Seattle is delayed.
Luke Marsden: And it's like, oh my God.
Luke Marsden: So.
Leah Smith: But yeah, let me message Tony and see if he's joining.
Luke Marsden: Yeah, cool.
Luke Marsden: Yeah.
Luke Marsden: So I just added the Find OS bot to your Slack channel.
Leah Smith: Oh, lovely.
Leah Smith: I'll be able to see it.
Leah Smith: I'm just on Slack now.
Luke Marsden: Yeah, you should be able to see it.
Luke Marsden: The.
Luke Marsden: The goal of this call will be to try and make it do something.
Leah Smith: Whereabouts would it be?
Leah Smith: On the.
Leah Smith: On the Slack interface.
Luke Marsden: Oh, it.
Luke Marsden: You should just see that it's just joined like the Helix Find AI channel or whatever that one's called.
Leah Smith: Oh God, yeah, sorry, I did see.
Leah Smith: Oh, was that the 1 that was 3:49am?
Luke Marsden: No, no, that was a pull request.
Luke Marsden: That was something random.
Luke Marsden: You can ignore that.
Leah Smith: Oh yeah, I've seen it now.
Leah Smith: Sorry.
Luke Marsden: Yeah, no, don't worry.
Luke Marsden: I mean it hasn't done anything yet, so I wouldn't have expected you to see it.
Luke Marsden: But um, yeah, do you want to share your screen and we can maybe try using the latest and just do some testing together?
Leah Smith: Desktop.
Leah Smith: No, or Slack.
Luke Marsden: Yeah, I do share the whole desktop maybe because I'll need you to use the browser as well.
Luke Marsden: Sorry.
Leah Smith: Okay.
Leah Smith: That my doing.
Luke Marsden: Do the whole thing.
Leah Smith: Desktop share.
Leah Smith: There we go.
Leah Smith: Can you see it now?
Luke Marsden: Yeah, yeah, yeah, that's great.
Leah Smith: Yeah, an event on Thursday so I created a video for the intro slides.
Luke Marsden: Oh, that's nice.
Leah Smith: I'm not a video editor by any means, but.
Leah Smith: Yeah, it took a while but got there eventually.
Luke Marsden: Yeah, well done.
Leah Smith: Really hard video editing have any idea how to do it?
Leah Smith: It's.
Luke Marsden: I'm definitely an amateur video editor as well.
Luke Marsden: Screencasts and stuff.
Leah Smith: But I. I know what I want.
Luke Marsden: Yeah.
Leah Smith: When things come in and then the logo's animated but I feel like that's, you know, people have proper software for all that kind of stuff.
Leah Smith: I. Yeah, Canva cap cut is my limit, I think.
Luke Marsden: A friend of mine has been making some rap music where he's fully generating the.
Luke Marsden: Both the audio and the video using AI and it's actually not bad.
Leah Smith: Wow.
Tony: Hey, Tony, are you all right?
Luke Marsden: Yeah, good, thanks.
Luke Marsden: How are you?
Tony: Yeah, not too bad, thank you.
Tony: Not too bad.
Luke Marsden: Good weekend?
Tony: Yeah, it's good, yeah, yeah, just with the kids really down the beach and stuff.
Tony: Yeah.
Tony: Oh, cool.
Luke Marsden: Yeah, I had Chris.
Luke Marsden: I had Chris with me.
Luke Marsden: We had a massive dinner party on Friday night.
Luke Marsden: Oh, it's a good fun.
Luke Marsden: Yeah, yeah, yeah.
Tony: You guys get together often or.
Luke Marsden: I was in Oregon like three months ago and he was here for a week just now.
Tony: Oh, nice.
Luke Marsden: So that's the first time we'd done that.
Luke Marsden: So we were joking about calling it like a cultural exchange.
Luke Marsden: Anyway.
Luke Marsden: No, it's good.
Luke Marsden: Um, so, yeah, um, yeah, I think I was just gonna get Leah to walk through the latest bits to test.
Luke Marsden: I wondered if you had any questions or, or topics first, Tony, because.
Tony: I know, No, I think the, the kind of agent thing.
Tony: So I did the test review last week and I haven't really done anything with it since, but probably just need a solution for.
Tony: For that.
Tony: I don't know, setting up like a dummy.
Tony: I mean, it's not ideal, but setting up like a. Yeah.
Tony: Fake person that we could then just have the agent being that fake person almost.
Luke Marsden: Yeah, yeah, No, I wanted to talk to you about exactly the same thing because like the.
Luke Marsden: There's definite risks from using the AI automation and we kind of stumbled across that together last week and, and I've been thinking a lot about how to de.
Luke Marsden: Risk it because I, I agree it's.
Luke Marsden: It would be like completely unacceptable for anyone's LinkedIn account to get like disabled or LinkedIn recruiter access to be disabled because that would be counterproductive.
Luke Marsden: Significantly counterproductive.
Tony: Yeah, exactly.
Luke Marsden: I do have some suggestions.
Luke Marsden: So I think using a separate LinkedIn account is one option.
Luke Marsden: I think the, the issues with that would be around.
Luke Marsden: It's more likely that that account gets flagged as automation because it's a new account.
Luke Marsden: So it's not.
Luke Marsden: It doesn't have the history and it would probably be less effective because it doesn't have the connections that, that your team have built up over time.
Luke Marsden: There are a couple of other ideas I have, but like, feel free to disagree.
Luke Marsden: Obviously one of them.
Luke Marsden: So I looked into kind of the various different signals that LinkedIn looks at in order to judge whether usage is automated or not.
Luke Marsden: And this is kind of a gray area in terms of the terms of service, if I'm completely honest, because it's like it isn't because it is actually a human logging in and it is a human using automation to do more of what they would normally do.
Luke Marsden: So I wouldn't call it fully automated, I would call it sort of semi automated.
Tony: Yeah.
Luke Marsden: So there's also I like wide sort of acceptance that some automation is accepted on the platform even though it's not officially recommended because there's so many different LinkedIn tools for like doing automatic LinkedIn connections with people like Dripify and yeah.
Luke Marsden: Alter and things like this.
Luke Marsden: So it, I think the, the game that, that people play with the LinkedIn platform tends to be around.
Tony: Are you,.
Luke Marsden: Are you being fair and are you paying them something?
Luke Marsden: And for paid accounts they generally like, they have higher thresholds for, for what they tolerate.
Luke Marsden: And the other thing that, that's important to bear in mind is that they don't just ban you outright, they will always give you some warnings before you get banned.
Luke Marsden: So the first one is they'll start showing you captures like we saw when you logged in.
Luke Marsden: I remember a big chunk of our call last week was you clicking on bicycles or something.
Luke Marsden: So that's the signal.
Luke Marsden: Okay.
Luke Marsden: They think something's up.
Luke Marsden: Like, okay, let's slow it down, let's step back from it for a minute, reassess.
Luke Marsden: And the other one was on LinkedIn recruiter in particular because they really care about the per seat licensing.
Luke Marsden: They don't want people sharing seats.
Luke Marsden: They do have fairly aggressive like, do you have this like simultaneous activity on the same seat from multiple IP addresses or multiple locations?
Tony: Yeah.
Luke Marsden: And then the third thing is making sure that the behavior is human.
Luke Marsden: Like which we can do with, with the way we prompt the agent.
Luke Marsden: So all of that said, my suggestion would be because I also looked into like some of the other signals that they use to, to detect automation.
Luke Marsden: One of them is circadian rhythm.
Luke Marsden: So if it looks like the agent is, or it looks like the user is never sleeping, then LinkedIn is like, Aha, that must not be a human.
Luke Marsden: Because humans do have to sleep some of the time.
Luke Marsden: And the other thing I thought was because we have this requirement that basically the human and the agent can't use the same account at the same time.
Luke Marsden: My suggestion is that we very carefully try doing a, what I would call a late shift.
Luke Marsden: So after, when someone is finishing work for the day, they could kick their agent off maybe it's to start at like 7pm and work until 10 or something like this.
Luke Marsden: And that way it looks like a human is just working late, but they're not working all the way through the night and they're doing enough useful work in that period that they can.
Luke Marsden: That the agent can then deliver them something by the time they get to their desk at like 8 or 9 or 10am the following morning.
Luke Marsden: So.
Luke Marsden: Yeah.
Luke Marsden: What, what, what do you think about that?
Tony: Yeah, I mean, I'm happy to explore any, any kind of ideas, really.
Tony: The only problem with the late shift is.
Tony: Is two different IP addresses, isn't it?
Tony: Which, which seem to be the problem because obviously I'm locked in it multiple places.
Tony: You know, at the moment I've got my desktop behind my laptop, so I always use both.
Tony: I just use this for calls and it's fine, it's never a problem.
Tony: Even though I kind of flick between the two all day long.
Luke Marsden: Yeah.
Tony: I think the IP address thing is.
Tony: But then I use this laptop in the office as well.
Luke Marsden: Yeah.
Tony: So I assume that's a different IP address.
Luke Marsden: Is a different IP address.
Luke Marsden: And if you think about a usage pattern where, like.
Luke Marsden: I mean, it's a bit of a weird idea, but like, imagine I took my laptop down the pub and jumped on the pub.
Luke Marsden: WI fi.
Luke Marsden: I did a bit of work in the evening while I was chatting with my mates.
Luke Marsden: I don't know if I actually would do that.
Luke Marsden: I have done that once or twice.
Tony: Yeah.
Luke Marsden: Or like if.
Luke Marsden: Just if you're on your.
Luke Marsden: If you're on your phone and you're doing stuff and then you're on a.
Luke Marsden: On a different mobile IP.
Tony: Would LinkedIn know that?
Tony: Would.
Tony: Would LinkedIn almost like name your index your device as well?
Tony: So it knows because sometimes it does say login on a new device.
Tony: Is it?
Luke Marsden: Is it?
Luke Marsden: Yeah, yeah, yeah.
Tony: Would LinkedIn know that?
Tony: Oh, that's Tony's laptop.
Tony: He's always on that kind of device.
Luke Marsden: I think it would, but then the, the, the desktop environment that the agents run in is like, it's Chrome on Linux and it's not.
Luke Marsden: And, and that is a valid, like, device type.
Luke Marsden: Yeah, it doesn't show up as like.
Tony: I just mean, like, it's a new device type for.
Luke Marsden: Yeah, yeah, yeah.
Luke Marsden: It would show up as a new device type, but then that session would survive as well.
Luke Marsden: So if LinkedIn is trying to kind of profile that user, it would look like they just got a Chromebook or something, like a new laptop.
Luke Marsden: So.
Luke Marsden: And again, we can kind of test this by seeing, well, oh, do we start getting captchas like we did last week?
Luke Marsden: Because you did validly log in.
Luke Marsden: Like we did get caught out last week because you were logged in from two different places.
Luke Marsden: Anyway, so the other thing I wanted to offer was while we're, while we're testing this and refining it, I'm happy to use my LinkedIn account which has a pretty good network and kind of take, take the risk myself.
Luke Marsden: And, and the other thing there is that the LinkedIn recruiter seat that I have from you is only for me.
Luke Marsden: So if that one gets like shut down, it won't affect any of the rest of your team.
Tony: But do you mean like do some outreach from.
Tony: Or just build product?
Tony: Because actually what you could do is build a project, put people into that and share the project with me.
Tony: So actually the agent does the sourcing and puts them into the project and then I just go in the next morning and go message all of these people.
Luke Marsden: Exactly.
Tony: Thank you.
Tony: Message.
Tony: Is that what you were thinking?
Luke Marsden: Yeah, exactly.
Luke Marsden: Yeah, yeah, yeah, I wasn't thinking that, but that's a really good idea.
Luke Marsden: Yeah, I was thinking something simpler than that, which was just that yeah, you could use my account to find people but not actually message them from me because it probably is a bit weird if I actually start recruiting for you.
Luke Marsden: But, but yeah we can, we can do all of that grunt work of a finance.
Tony: That's a workaround for.
Tony: Because the only thing about, you know, setting up a new pro, like a dummy profile.
Luke Marsden: Yeah, it's.
Tony: If it's, you know, John Smith started at Linux recruit yesterday.
Tony: Zero connections.
Tony: If so if that emails people, they're gonna think, you know, there's no credibility with that profile.
Tony: Message me.
Tony: But if we get John Smith to actually just do the building the project work and then actually they share that with one of the team and then yeah, or me or whatever and I just go in and so that's maybe a workaround.
Luke Marsden: Yeah, yeah, yeah, no, I think that's good.
Tony: Yeah.
Luke Marsden: And like I say in the next, like through, through the next like month or so of the project, we've got like two months left, haven't we?
Luke Marsden: I'm going to say we.
Luke Marsden: Yeah, two months left, maybe slightly less, but I'm not really counting the,.
Leah Smith: The.
Luke Marsden: Yes, if we, if we use my account for it, at least for the next month or so while we get more comfortable with it, then, then we'll see how your comfort levels are in terms of either using your individual accounts or using a dummy John Smith account when we want to roll it out to the rest of the team.
Luke Marsden: Does that sound okay?
Tony: Yeah, I think that's.
Tony: That sounds.
Tony: I mean, the dummy account could actually work really well because that could just be almost like our.
Tony: We give the AI one of our licenses.
Luke Marsden: Yeah, yeah, yeah.
Tony: And you know, if that gets banned, then fine.
Luke Marsden: Yeah, yeah, no, that makes sense.
Luke Marsden: And again, I think it won't get banned straight up, straight away.
Luke Marsden: It.
Luke Marsden: You'll get a few kind of warnings and so we can back off if, if it looks like we're getting, getting anything risky going on on that account.
Luke Marsden: And then there's other things we can try as well, like non LinkedIn ways of sourcing and like.
Luke Marsden: Yeah, yeah, yeah.
Tony: You know, GitHub and probably the job boards, our database.
Tony: We haven't, you know, plugged into Bullhorn as such to search that and.
Luke Marsden: Yeah, yeah, yeah, definitely.
Luke Marsden: Yeah.
Tony: Because we should be able to set it up so sources Bullhorn and maybe sends them emails or automatically puts them into like a vacancy shortlist that the.
Tony: We then have to go into.
Tony: And so we can set it up overnight to shortlist the 15 best people for this job or whatever, and then morning, the guys can duck in and that's 15 calls for them to make straight away kind of thing.
Luke Marsden: 100%.
Luke Marsden: Yeah, yeah, yeah, yeah.
Luke Marsden: I love that.
Luke Marsden: Okay, cool.
Luke Marsden: And we won't have similar problems with Bullhorn because, like, we've got API access there, so.
Tony: Yeah, exactly.
Luke Marsden: And you're paying for it.
Luke Marsden: You're using it in the exact legitimate way it was intended.
Tony: Yeah.
Luke Marsden: So awesome.
Luke Marsden: Cool.
Luke Marsden: Okay.
Luke Marsden: Are you comfortable with that then?
Luke Marsden: I don't want to triple check.
Tony: Yeah, if you're, if you're happy to do that or like I said, I'm happy to set up another, you know, a fake account or we, we probably should set up that.
Tony: If we're going to do a fake account, we probably should do that within the AI.
Tony: So then the IP address is always there.
Luke Marsden: Yeah.
Tony: Does that make sense?
Luke Marsden: Yeah, yeah, yeah.
Tony: Profile in there, add all the info and add all the.
Tony: Yeah, Jazz.
Luke Marsden: Yeah, I, I think that makes sense.
Luke Marsden: I'm a little worried about the fake accounts because I think they're likely to get flagged much more easily.
Tony: Yeah.
Luke Marsden: I almost wonder if there's anyone else in or around the business that isn't directly doing outreach all the time, but would be willing to like, donate their LinkedIn account.
Luke Marsden: It's a bit of an ask.
Tony: I mean, there, there would have been six months ago, but we don't really have anyone kind of doing operations admin at the moment.
Luke Marsden: Yeah.
Tony: So Leah's obviously ending up marketing and picks up some of the op stuff because there isn't a huge amount these days.
Tony: But obviously Leah uses her LinkedIn for.
Tony: For stuff.
Tony: Yeah, yeah, yeah, yeah and I will.
Leah Smith: Obviously, yeah I'll start probably using it for maybe recruitment purposes filling its recruit soon as well.
Luke Marsden: Yeah, maybe if that's cool.
Luke Marsden: That makes sense.
Luke Marsden: And it's quite personal thing to ask of people.
Luke Marsden: Can we, can we use your LinkedIn account for outreach but for automation I mean.
Luke Marsden: Okay, cool.
Luke Marsden: So I think there is, I don't know, I want.
Tony: There's probably, you know, I've got, I've got friends who don't ever use LinkedIn profile.
Tony: So I don't know if you know, if I said to them, look, would you mind us borrowing your LinkedIn profile, changing you to an employee of Linux recruit started two years ago.
Tony: Yeah, we could easily do that.
Tony: And then.
Tony: So let me see if there's a.
Luke Marsden: Cool.
Tony: Find someone there might be a few.
Luke Marsden: Pints or a dinner in it.
Tony: Yeah, exactly.
Luke Marsden: Because I know that like farming LinkedIn accounts is a whole industry, but it's a bit of a dodgy industry that you don't really want to get involved with.
Luke Marsden: So.
Luke Marsden: But yeah, I don't know in your budget.
Leah Smith: So yeah,.
Luke Marsden: I get it all the time but minor mine are less subtle than that because you knocked.
Tony: I couldn't believe it.
Luke Marsden: Amazing.
Luke Marsden: Yeah.
Tony: And obviously the, the website.
Tony: I've just gone onto the website and it's not working.
Luke Marsden: Oh really?
Tony: Oh yeah, it's saying service is starting up, be ready in a moment and then doesn't load for some reason but.
Luke Marsden: Oh, let me.
Luke Marsden: Yeah, we should have got alerted to that.
Luke Marsden: Sorry about that.
Luke Marsden: I will fix that straight after the call.
Luke Marsden: So let's try the.
Luke Marsden: Let's.
Luke Marsden: Let's try out the latest then.
Luke Marsden: Who wants to drive?
Leah Smith: I can drive.
Tony: Okay.
Leah Smith: Desktop.
Leah Smith: Did you say the whole desktop?
Luke Marsden: Yeah, if you could show your whole desktop that'd be great.
Luke Marsden: Cool.
Leah Smith: Sorry it's a bit of a mess.
Leah Smith: My desktop.
Luke Marsden: Don't apologize.
Luke Marsden: Right, yeah.
Luke Marsden: So if you go to os, we find AI with a dot.
Leah Smith: How did that not go in there?
Leah Smith: Dot.
Luke Marsden: Sorry, dot.
Tony: AI.
Luke Marsden: That's it.
Luke Marsden: Okay, cool.
Luke Marsden: So actually that's a bit weird.
Luke Marsden: It just says user in the top, right?
Luke Marsden: I saw this earlier.
Luke Marsden: Try clicking sign out and then sign in with Google.
Leah Smith: Would it be.
Luke Marsden: I mean probably your.
Luke Marsden: I mean to be honest.
Luke Marsden: Well actually no login.
Luke Marsden: Log into this with your Leo Smith with the top one.
Luke Marsden: Yeah, yeah, because this is just logging into to the app.
Luke Marsden: Yeah.
Luke Marsden: And then if you go to candidate search and then pick one that you actually want to do a search for, we should now support uploading files.
Luke Marsden: So let's start with files that I don't know if we have the file.
Luke Marsden: Well, the.
Luke Marsden: Tony emailed me a whole load of these.
Luke Marsden: Shall I forward them to you or.
Leah Smith: Yeah, if that'd be good.
Tony: Is this a search?
Tony: Another search kind of demo?
Luke Marsden: Yes.
Luke Marsden: Yeah, this will be sourcing.
Luke Marsden: It's just I've done a whole load of work to make it more robust and.
Tony: Nice.
Luke Marsden: Picked some bugs that we saw last time.
Luke Marsden: So I wanted to test.
Luke Marsden: Go through testing it.
Leah Smith: Are you forwarding it on Slack, Luke?
Luke Marsden: I. I'm actually wondering.
Luke Marsden: It might be quicker for me just to upload.
Luke Marsden: Upload them myself.
Luke Marsden: Just going to do that now.
Tony: I'm just going to share screen then.
Luke Marsden: Yeah.
Luke Marsden: Should I share my screen so you can see?
Leah Smith: Should I stop sharing mine?
Luke Marsden: No, it's okay.
Luke Marsden: We can allow multiple presenters at the same time.
Tony: Oh really?
Luke Marsden: How fancy is that?
Luke Marsden: Yeah.
Luke Marsden: So here I went up to Eric.
Luke Marsden: I'm just going to grab all of these.
Tony: And the.
Tony: The website has been.
Tony: Any progress on that or is that still as is?
Luke Marsden: That's as is.
Luke Marsden: Yeah.
Luke Marsden: No, I've been working on the sourcing.
Tony: Yeah.
Tony: Okay, cool.
Luke Marsden: Based on kind of following along from what we did last time.
Luke Marsden: So what I'll do here.
Luke Marsden: So I'm in as me.
Luke Marsden: Let me just log out and log in again.
Luke Marsden: That one.
Luke Marsden: There we go.
Luke Marsden: So I'm in here with my Linux recruit email.
Luke Marsden: I can go into conduct AI and then.
Leah Smith: So.
Luke Marsden: I'll just do these.
Tony: There we go.
Luke Marsden: So that's uploaded a whole load of context.
Luke Marsden: That's general across all of them.
Luke Marsden: Although actually we should probably get rid of these specific roles because they're doing Product Engineer and backend engineer as two separate roles.
Tony: Yeah.
Tony: Yeah.
Tony: So I'd probably just leave one of them maybe.
Luke Marsden: Well I'll take both of them out because then we can put them in individually because this is.
Luke Marsden: These are files across the whole client.
Tony: Right.
Tony: Okay.
Luke Marsden: And the other ones are so that.
Tony: That the top two were like audio files.
Tony: So would it take the audio then?
Luke Marsden: Yeah, so actually the agent was smart enough last time I tried to use the GPU that it has access to to transcribe the audio automatically.
Luke Marsden: Oh great.
Luke Marsden: So that was pretty impressive.
Luke Marsden: So let me just grab these.
Luke Marsden: This is Product Engineer AI native.
Leah Smith: Do I need to refresh the page or shall hold fire at the minute?
Luke Marsden: If you hold fire for a second.
Luke Marsden: And then the Other one was.
Luke Marsden: Backend engineer, AI native.
Luke Marsden: We can add those roles and then when you go into a role, you can then upload the specific bits for that role.
Luke Marsden: So.
Luke Marsden: So here we've got.
Luke Marsden: We've uploaded the document there and the product engineer, there's another document here.
Leah Smith: I can't see your screen.
Leah Smith: Is that because I'm sharing?
Luke Marsden: Oh, yeah, possibly.
Luke Marsden: I think if you look at Zoom, there's a tab at the top that will show my screen.
Luke Marsden: If you click on it, it's not very obvious or you can stop sharing.
Leah Smith: So I just noticed that.
Luke Marsden: Okay, so I've uploaded.
Luke Marsden: Can you see now, Leah?
Luke Marsden: Yeah, yeah, yeah, Great.
Luke Marsden: Okay, so now we've got all the files for conduct here.
Luke Marsden: And then we've got the two specs.
Luke Marsden: We've got the two roles here and that's got the Docx file there.
Tony: So for each company we can have like a shared area where we put in different bits and pieces of info.
Tony: Is that.
Luke Marsden: Yeah, exactly.
Luke Marsden: So this is anything that's general to conduct.
Tony: Yeah.
Luke Marsden: And then within each job spec, you can put anything that's specific to that job that JD So then once you're in here.
Luke Marsden: Well, actually, yeah.
Luke Marsden: Do you want to share again, Leah?
Luke Marsden: So I actually still are.
Luke Marsden: So now you should be able to go hit refresh.
Luke Marsden: A good live test.
Luke Marsden: Okay, cool.
Luke Marsden: So you can see that now those are the bits I uploaded.
Luke Marsden: And then if you go into one of them at the bottom.
Luke Marsden: Sorry, the.
Luke Marsden: One of the job bags.
Leah Smith: Sorry, my mouse is there like a. Oh, it's there.
Leah Smith: Sorry, I couldn't see the draggy thing.
Luke Marsden: Ah, yeah, no worries.
Luke Marsden: Yep.
Luke Marsden: Then you can see we've got the Docx file there.
Luke Marsden: So now if you try run sourcing.
Luke Marsden: So now we start getting a bit strict with people here.
Tony: Okay.
Tony: All right.
Luke Marsden: You might want to search your name in there because there's.
Leah Smith: Oh, is that me?
Luke Marsden: Yeah, I hope so.
Leah Smith: Do I need to tick?
Luke Marsden: Yeah, but you need to actually do it as well.
Leah Smith: Look out, I've LinkedIn.
Leah Smith: You're out there.
Leah Smith: So is that on the phone as well?
Luke Marsden: Probably, or just don't use it on your phone.
Luke Marsden: But it does say log out on your phone.
Leah Smith: Logins are.
Luke Marsden: Tony.
Luke Marsden: This is the sort of safety stuff that should hopefully stop a recurrence of what happened last week.
Leah Smith: Sign out of the one I'm logged into.
Luke Marsden: Cool.
Luke Marsden: Okay, so give that a second.
Luke Marsden: And if you scroll up just a little bit, you'll notice there's that little warning message as well.
Luke Marsden: Not there.
Luke Marsden: Sorry, the whole.
Luke Marsden: In the.
Luke Marsden: Oh, wow.
Luke Marsden: Look, it's Uploaded all those files.
Luke Marsden: Okay, cool.
Luke Marsden: Yeah, that's it.
Luke Marsden: I just wanted to point out that there's also that little warning sign.
Luke Marsden: If you scroll up the whole page a little bit, it also says, this agent is live on your LinkedIn seat.
Luke Marsden: Don't open LinkedIn Recruiter until the run ends.
Luke Marsden: So the idea here, by the way, is that the hierarchy is like clients, and then within clients you've got jobs, and then within jobs you can have multiple agents running on behalf of different people because you will have multiple team members that might be working on a job.
Luke Marsden: And so each team member will be logged into their own LinkedIn.
Luke Marsden: Ultimately, if we can get that all working safely, which I think we will.
Luke Marsden: And.
Luke Marsden: And yeah, so it's.
Luke Marsden: So that's kind of the.
Luke Marsden: The hierarchy.
Luke Marsden: Does that make sense and does it sound like the right structure?
Tony: Yeah, yeah, definitely.
Luke Marsden: Okay, cool.
Tony: Similar to last week, but actually it's got the consideration for LinkedIn and also the uploads working as well.
Luke Marsden: Exactly.
Luke Marsden: And also this idea of having multiple people working on one job, which separate.
Luke Marsden: Which before we didn't.
Luke Marsden: We didn't have.
Luke Marsden: We just had one agent session per job, which would have not made sense if multiple people were working them at the same time.
Luke Marsden: Yeah.
Tony: So, yeah, yeah, it looks good.
Tony: I think we mentioned about hiding a lot of the stuff on the left as well.
Luke Marsden: Yes.
Luke Marsden: To make it a bit.
Tony: Definitely be.
Tony: Because I think with.
Tony: With these kind of tools to give to the team, like if there's less stuff to look at and click on, the better.
Tony: Normally.
Luke Marsden: Yeah.
Luke Marsden: It is a bit hectic at the moment.
Luke Marsden: I hear you.
Luke Marsden: So, yeah, hiding the.
Luke Marsden: Actually probably.
Luke Marsden: Yeah.
Luke Marsden: Just hiding the whole sidebar when you've gone into a job.
Leah Smith: Yeah.
Luke Marsden: Making it full screen.
Tony: Okay, cool.
Tony: So what.
Tony: What do we need to kind of do from here?
Tony: Just come up with a strategy for the LinkedIn stuff, but we need to decide on what we're going to do.
Luke Marsden: Yeah, I did just want to watch this for another minute.
Tony: Okay.
Luke Marsden: So.
Luke Marsden: Because in a minute it's going to ask Leah to log in and what it should also do, and this is something that I wanted to test with your real Slack users, is it should ping you on Slack to ask you to log in.
Luke Marsden: And like I said, for testing after this, I'm happy to log in for you and actually I'll run it on my account and actually try and get you some lists.
Luke Marsden: So literally you.
Luke Marsden: At this point I'm like, you tell me what jobs you want lists of candidates for and I'll actually try and deliver the goods Using an agent.
Tony: I think the.
Tony: Can you access all the MER stuff that I uploaded last week?
Luke Marsden: Let me try.
Tony: Yeah, I ran that search last week.
Tony: Can you?
Luke Marsden: Oh look Leah, you just got pinged on Slack.
Leah Smith: Oh yes.
Leah Smith: So I was just trying to work out what my Password is to LinkedIn.
Luke Marsden: Look, it pinged you.
Tony: Okay, cool.
Luke Marsden: And what you'll see there.
Luke Marsden: Try clicking.
Luke Marsden: So assume that we get these things to run automatically at like 6pm now every 6pm you'll get a ping so you won't have to remember to kick it off yourself.
Luke Marsden: And then the Slack ping will give you the link.
Luke Marsden: Try clicking the link.
Luke Marsden: No, the second one.
Luke Marsden: Sorry.
Luke Marsden: Jump in here.
Luke Marsden: Oh, that's interesting.
Luke Marsden: That link is broken so I need to fix that.
Luke Marsden: So thank you for testing it.
Luke Marsden: I'll fix that.
Luke Marsden: But it will in.
Luke Marsden: The idea is it will jump you to there.
Luke Marsden: Exactly.
Luke Marsden: That that tab you were about to click on, it should jump you there.
Luke Marsden: So yeah.
Luke Marsden: Now if you could try logging in that would be great.
Luke Marsden: And again we'll be exceptionally careful not to trigger anything here.
Luke Marsden: Okay.
Luke Marsden: Ah, captures.
Luke Marsden: Interesting.
Leah Smith: Let me just make this false.
Tony: Leah, have you logged out of your LinkedIn recruiter elsewhere?
Leah Smith: Yeah, yeah, yeah.
Luke Marsden: Oh, you know one thing.
Luke Marsden: Do you not have two factor auth turned on?
Leah Smith: I should do?
Leah Smith: Yeah, yeah I do because it asked me when I log in at other places.
Leah Smith: Like it sends me an email.
Leah Smith: That's weird.
Leah Smith: It's just logged me out.
Luke Marsden: Yeah, that's a bug that I tried to fix earlier.
Luke Marsden: I guess it didn't actually.
Luke Marsden: Just tell the agent for a second stop refreshing the page.
Luke Marsden: And hit enter.
Luke Marsden: Yeah, yeah.
Luke Marsden: Oh, you knew how to do that?
Luke Marsden: Yeah, you hit enter twice, didn't you?
Luke Marsden: Okay, cool.
Luke Marsden: Yeah.
Luke Marsden: So try again.
Luke Marsden: I think it's okay to fill in one captcha, But that is sort of definitely one.
Luke Marsden: Oh, now it wants to check to a phone number.
Tony: That's good.
Luke Marsden: Yeah, there actually a two factor.
Luke Marsden: And it's saying recognize this device in future, which is kind of nice.
Luke Marsden: Okay, great.
Luke Marsden: And you're in.
Leah Smith: I'm in.
Luke Marsden: So we didn't have to click on the cars in the end.
Tony: Yeah, that was better than last week.
Luke Marsden: Yeah.
Luke Marsden: Okay, so now tell the agent I'm in.
Tony: So Leah, you.
Tony: You don't necessarily need your personal LinkedIn as much as maybe we do, do you?
Tony: Your LinkedIn recruiter.
Tony: It's very rare you're in LinkedIn recruiter, isn't it?
Tony: Well, almost never.
Leah Smith: Yeah, I mean aside for I message people about the events, but that's it.
Tony: Yeah, but you can do that from your normal LinkedIn, you can just connect with them and then.
Tony: Yeah, because you, you could unleash your LinkedIn recruiter now to build a project for conduct for, you know, the next however many hours and then once it's done, you can just go in there in the morning and share that with me and then I can send the.
Tony: Or share it with like Ethan or share it Alex.
Tony: Yeah, if it, if it works.
Leah Smith: Yeah,.
Luke Marsden: Yeah, that's, that's cool.
Luke Marsden: I mean, if Leah's comfortable with it.
Leah Smith: So what do.
Leah Smith: I, am.
Leah Smith: I just.
Leah Smith: Will I just set up obviously the projects on my LinkedIn recruiter and just share it with whoever.
Tony: This will serve the project itself.
Leah Smith: Yeah.
Tony: So this, this will set up a project, but then once it's, once the project's.
Tony: Once it's allocated a bunch of people to the project, then you just go into the project on your own.
Leah Smith: Yeah.
Tony: Or you can just tell the bot, tell the AI, whatever we want to call it.
Tony: Yeah.
Tony: To share that project with Tony or share it with Alex or.
Tony: And it will do the fullest, won't it?
Luke Marsden: Yeah, yeah, I think that's a good idea.
Luke Marsden: And do you.
Luke Marsden: I think the only thing, the only question I have there is about.
Luke Marsden: Well, I guess on a case by case basis you'll know whether you've already got a project set up for a given job or it doesn't even matter.
Luke Marsden: I guess if you get duplicate projects.
Tony: No, we, yeah, we can have duplicate.
Tony: As long as we're not sending out duplicate messages.
Tony: But I think we've kind of set some rules.
Tony: If they've been contacted or if they've replied to an InMail within a certain period, then don't send them another one.
Tony: Is that.
Tony: But we can also, we can also sanity check that, you know, if it puts 50 people in a project overnight and I can just manually go through those 50, that's a lot easier for me than going through 3.7 million likes just come up on here.
Luke Marsden: Yeah.
Tony: You know, still, it's still giving me a short, a long list kind of thing.
Tony: It's not.
Tony: And I can, I can say manually say yeah or no less messages for now, let's discount them kind of thing.
Tony: So I can just go through 50 really, really quickly rather than.
Leah Smith: So it'll just be me logging into.
Leah Smith: So it's not like the whole team are going to log on to.
Leah Smith: The whole team.
Leah Smith: Going to log on to my LinkedIn recruiter.
Leah Smith: Or is it just me setting up the projects and then passing it over to them?
Tony: No.
Tony: So this, this is all Being done behind the scenes.
Tony: You don't have to do anything, Leo, apart from tell the AI what you want it to do.
Tony: And then the guys don't need to log into your LinkedIn.
Tony: We can just share that project.
Tony: So then as soon as you share it, it goes onto their own LinkedIn and they're in their LinkedIn so they've got what the, the kind of the.
Tony: The AI has done for them over that period of time.
Tony: Makes sense.
Luke Marsden: Yeah.
Luke Marsden: And there's also the possibility to draft inmails or messages to those people, which would also be a huge time save.
Tony: Yeah.
Luke Marsden: Because the agent does quite a good job of like getting those right.
Tony: Yeah.
Luke Marsden: Or getting them close enough and people can do a little edit.
Tony: The only problem with sending them those.
Tony: It would have to come from Leah, wouldn't it?
Luke Marsden: Or.
Luke Marsden: Well, no, we can figure out a way to in find OS that is this interface to allow your team to log in and get a list of.
Luke Marsden: Of those drafts and then they can either just copy paste them one by one and at some point they'll be like, oh, can I get an agent to send them for me?
Luke Marsden: At which point we get back into the.
Luke Marsden: Everyone is logged in on their own accounts in a very careful way.
Tony: So I think, I think that at the moment is.
Tony: We just have to.
Tony: If you want to be quick about it, if the, you know, this has built up a project of 30 and they're all really good, we'll just have to send a personalized email to all of them in one go.
Tony: I think would probably be the most efficient way.
Tony: So that all that.
Tony: All the personalization would be.
Tony: Would be the first name probably.
Luke Marsden: Oh yeah, just send the same note to everyone.
Tony: Yeah, yeah, yeah, yeah.
Tony: Which is not.
Tony: Which is the most ideal but I think at the moment, while we can't log in from two different places at the same time.
Luke Marsden: Yeah, no, that makes sense.
Luke Marsden: And I wonder when you put a candidate into a project in LinkedIn Recruiter, can you also set up a draft in mail for them or does it not work?
Tony: So you can set up draft templates.
Tony: Yeah, that, that's it.
Leah Smith: The templates.
Leah Smith: I don't know if you can share templates with.
Tony: Yeah, you can share templates, that's fine, but it wouldn't be.
Tony: Well, yeah, you could.
Luke Marsden: Well you could actually just have a template per every candidate.
Luke Marsden: Yeah, candidate and then that's kind of aware.
Luke Marsden: A way of sharing the draft.
Tony: Yeah.
Luke Marsden: Without copy pasting.
Luke Marsden: I do think finding ways to reduce copy pasting is probably.
Tony: Yeah, yeah, the move.
Luke Marsden: So yeah.
Luke Marsden: Okay, that sounds promising.
Tony: Yeah.
Tony: Because it's set up a draft every time and then they just need to go on to that candidate, select the draft, send it.
Tony: But that is manually going through each one, sending the, the personalized draft.
Tony: So if there's 50 candidates, you've got to do that 50 times.
Luke Marsden: It's still 50.
Luke Marsden: It's still 50 times however many clicks.
Luke Marsden: Yeah, so you.
Tony: But what I mean is if you've got a template, you still gonna have to go it.
Tony: It's still going to be a manual process to do that.
Tony: Yeah, but whereas we could just go in and go message all bang.
Luke Marsden: Yeah, that makes sense.
Luke Marsden: So message all with the same message is probably the least friction.
Luke Marsden: Yeah, let's get something working.
Luke Marsden: Yeah, yeah.
Tony: I think if we're able to build a project and then able to put people into that project.
Tony: So they haven't they set up a project yet on this.
Tony: That's the next step, I think.
Leah Smith: Yeah.
Leah Smith: Just going through.
Tony: Yeah.
Tony: So Hubert,.
Luke Marsden: It seems to be doing the right thing in terms of noticing about the other people on the team having accepted things.
Luke Marsden: Okay, so at this point you don't.
Tony: Scroll up on the left hand side a bit a sec please.
Tony: Yeah, I just want to screenshot that quickly.
Tony: Down a little bit, down a little bit.
Tony: Stop.
Tony: Yeah, that's it.
Tony: Thank you.
Luke Marsden: So I guess the, the duplicates, you not want them in the project.
Tony: Do I?
Tony: I think if they've responded to a message, I would, I would say no.
Luke Marsden: Okay.
Luke Marsden: I don't know if we know whether they've responded.
Luke Marsden: Says no response yet.
Luke Marsden: Look in brackets there said if you.
Tony: Go up a sec.
Tony: Keep going, keep going.
Tony: Candidate accepted in mail.
Luke Marsden: Hold on.
Luke Marsden: Yeah, and you can see it, it's not very big, but you can see it in the actual desktop as well.
Luke Marsden: Like no response yet.
Tony: Oh, okay.
Tony: Oh, that's a different candidate.
Luke Marsden: Oh yeah, it's moved on to the next one.
Tony: Yeah.
Luke Marsden: Okay, well done.
Tony: So this one's fine.
Tony: Viewed by and sent in mail.
Tony: No response yet.
Tony: They should be fair game.
Luke Marsden: Okay, so we can tweak that because that's not the decision it's making right now.
Tony: I think, I think we said I've lost last time.
Luke Marsden: Yeah.
Luke Marsden: But it's doing the wrong thing.
Luke Marsden: So I'll take that as input and make and, and improve it.
Luke Marsden: Cool.
Luke Marsden: So this will be boring to watch at this point.
Luke Marsden: It might be worth leaving it.
Leah Smith: Yeah.
Luke Marsden: And then, and then coming back to it.
Tony: Is it definitely saving them to a project though?
Tony: Is that if we got to that point yet?
Luke Marsden: I would ask it yeah.
Tony: Can you ask it?
Tony: Yeah.
Tony: Are you saving these?
Tony: Do you have a project set up to save these two?
Luke Marsden: Oh, we found a new one clean on both DDUBE checks.
Leah Smith: Good question.
Leah Smith: No, that may be straight.
Tony: Yeah.
Tony: I think I am surprised not.
Tony: So what I've done is stage candidate.
Luke Marsden: I think it probably would have done that next actually no, don't be sorry.
Tony: I'm just found when we find a first candidate.
Luke Marsden: Yes.
Luke Marsden: Does it.
Luke Marsden: Yeah.
Luke Marsden: Yeah yeah, yeah.
Luke Marsden: I can't.
Luke Marsden: We could actually ask the agent what is the exact process you're following here.
Leah Smith: Looks going quite quick.
Luke Marsden: And the AI can type fast.
Leah Smith: Scored.
Tony: Yeah.
Tony: Okay.
Tony: We've not done third step 3D.
Tony: Save the project.
Tony: Yeah.
Tony: Okay.
Tony: No, create.
Tony: Create an existing one.
Tony: No, create a new one.
Tony: Sorry, not existing.
Leah Smith: Yeah.
Tony: Create and use a new project.
Leah Smith: Create a user new project.
Tony: Create and use a new project.
Luke Marsden: Nice.
Leah Smith: I like interrupting it.
Luke Marsden: I interrupt them all the time.
Luke Marsden: It also asks question two.
Luke Marsden: What do we think about that?
Luke Marsden: Save only the candidates on drafting outreach for also the DD skips.
Luke Marsden: Oh well we should.
Luke Marsden: We should nudge it rules.
Tony: Yeah.
Luke Marsden: Say.
Tony: So.
Tony: Add.
Tony: Add those we are drafting outreach for.
Tony: Do not add anyone who has replied to an InMail in the last three months.
Tony: Should we say that Le.
Tony: Sorry.
Luke Marsden: Yeah, whatever you think is appropriate.
Tony: Yeah, do that for now.
Luke Marsden: Okay, great.
Luke Marsden: But Sebastian was meant to get.
Luke Marsden: Oh, he in mailed less than three months ago but that's not quite right because that doesn't mean he never.
Luke Marsden: He has.
Luke Marsden: He has replied to an InMail in the last three months because he hasn't replied to any emails.
Luke Marsden: Hasn't replied to that email Anyway, it's a bit of a detail.
Tony: Not added.
Tony: Hold on.
Luke Marsden: Say.
Tony: Oh, weak fit.
Tony: Weak fit.
Tony: Okay cool.
Luke Marsden: Okay.
Tony: Dg skip.
Tony: What?
Tony: What we.
Luke Marsden: How's it staging the draft?
Tony: I think the draft in mail is low priority at the moment.
Luke Marsden: Yeah.
Luke Marsden: Yeah.
Luke Marsden: Because we were saying that we would just wanted them in the project and then hand them over to someone else.
Luke Marsden: That's fine.
Luke Marsden: Why don't you leave that with me.
Luke Marsden: I'll take all the feedback from this call and do another pass through it.
Luke Marsden: I suggest for now we leave this one running.
Luke Marsden: Maybe Leah, if you want to check in on it in an hour or something, kind of keep half an eye on it.
Luke Marsden: But I'm not too worried.
Luke Marsden: It seems to be doing the right thing.
Luke Marsden: And yeah, we can just chat a bit on Slack later or tomorrow morning about how.
Luke Marsden: How this has gone and at this point we're kind of.
Luke Marsden: I. I think the.
Luke Marsden: The one other thing we need to take from this is the workflow that we haven't got yet of telling the agent to then pass the project along to one of the teams.
Luke Marsden: Oh.
Luke Marsden: But yeah, you can see it's also populating in here as we go.
Luke Marsden: So that's pretty cool.
Luke Marsden: Okay, great.
Luke Marsden: I'll keep working on this then, folks.
Leah Smith: Will it let me know, Luke, once it's.
Leah Smith: Once it's finished, or should I go in in an hour and manually stop it if it will just keep going?
Luke Marsden: That's a good question.
Luke Marsden: I don't actually know.
Luke Marsden: So go in an hour just to check.
Luke Marsden: But.
Luke Marsden: Yeah, but you need to stop it in an hour.
Luke Marsden: No, I think you need to stop it.
Luke Marsden: It's just more of a check in on it in an hour and maybe let.
Luke Marsden: Let us know on Slack or let me know on Slack how it.
Luke Marsden: How it's going.
Tony: I wonder if I.
Leah Smith: If it.
Tony: When you could ask it when you set up a project.
Tony: Could you please share this with Tony Chapman?
Tony: And then I might be able to see the candidates being dropped in there live.
Luke Marsden: Yeah, that'd be good.
Tony: Which would probably be useful.
Luke Marsden: Yeah.
Luke Marsden: Yeah, that'd be great.
Tony: And then I can check that my project's in an hour.
Tony: Have I.
Tony: Or whenever.
Tony: Have I got any?
Luke Marsden: Yeah.
Leah Smith: Does he need to.
Leah Smith: Does he.
Leah Smith: Does the agent need to know?
Leah Smith: Tony Chapman, he'll know exactly who to share it.
Leah Smith: Share it to if I just.
Leah Smith: Or any more details.
Leah Smith: Does he need to know?
Luke Marsden: I think it should be fine because it'll probably be in your recruiter account.
Luke Marsden: You'll probably be able to see your teammates in there at a guess.
Tony: Yeah.
Luke Marsden: Tony, you probably know the UI better than I do.
Leah Smith: Yeah.
Tony: When you.
Tony: It's quite.
Tony: It should be pretty easy today.
Luke Marsden: Okay.
Tony: Yeah.
Luke Marsden: And it's doing the right thing now.
Luke Marsden: It's like, yeah, it's gonna do.
Luke Marsden: It's probably gonna do that.
Luke Marsden: And then carry on would be my guess.
Luke Marsden: Excellent.
Luke Marsden: All right, well, it feels like some progress today.
Leah Smith: Yeah.
Tony: Yeah, definitely.
Luke Marsden: Yeah.
Tony: Thank you for that.
Luke Marsden: Thank you both.
Luke Marsden: Yeah.
Luke Marsden: Let's keep chatting on Slack and if we can get.
Luke Marsden: I'm happy to have a few more touch points with you both this week on just like getting this to be actually useful for you as soon as possible because that was the vibe last week before the LinkedIn shenanigans was like, let's actually get some value out of this now and then.
Leah Smith: It's rate on it, just to be clear.
Leah Smith: So I obviously signed out of my phone and everything.
Leah Smith: If I'm at home, I can obviously sign back in on my phone and everything.
Leah Smith: Like that.
Leah Smith: Is that okay?
Luke Marsden: Should be totally fine.
Luke Marsden: I would also say.
Luke Marsden: Yeah, this was kind of why I was thinking get them to do the late shifts because, like, after you actually want to start working for the evening is the right time to kind of hand over your account to the agent to get it to do a night shift.
Luke Marsden: So you.
Tony: You kind of have to leave it now while the agent is doing its thing, though.
Tony: Leah, is that.
Leah Smith: Yeah, that's fine.
Leah Smith: I'm just thinking, like, can I not log into on LinkedIn on my phone then?
Leah Smith: Like for the meantime or just while the agents.
Tony: Ideally not.
Leah Smith: I think that's fine.
Leah Smith: I just didn't.
Tony: Yeah, you think that's right, Luke?
Luke Marsden: I. I would agree.
Luke Marsden: And again, this is why I'd say, like, do it in the evening when you actually want to stop working.
Leah Smith: So I can't use LinkedIn on my desktop if I needed to or for any reason, which is fine.
Leah Smith: I just.
Leah Smith: Just so I don't.
Luke Marsden: Maybe.
Luke Marsden: Yeah, just time.
Luke Marsden: Box it to an hour.
Luke Marsden: Say, like, stop the agent, turn it off, tell it to stop, tell it to log out or whatever.
Tony: Yeah.
Luke Marsden: Also, when the agent stops, naturally, it will then shut itself down an hour later.
Luke Marsden: That's the vibe.
Luke Marsden: So it's.
Luke Marsden: Then it's automatically logged out, or it's at least it's not doing anything at that point.
Luke Marsden: So then it's safe for you to.
Leah Smith: Log back in where it says restart desktop.
Leah Smith: That means that it's logged out and you're correct.
Luke Marsden: Exactly.
Luke Marsden: Yeah, yeah, yeah, yeah.
Luke Marsden: So kind of use that.
Tony: Use that as a signal to be like LinkedIn to.
Tony: Just not LinkedIn recruiter.
Luke Marsden: It's probably okay, but I think at this point we probably want to err on the side of caution for a minute.
Luke Marsden: But again, if.
Luke Marsden: If, Liv, you've got something you need to do on LinkedIn this afternoon, then.
Leah Smith: No, no, I can.
Leah Smith: For an hour or so, I can be without it.
Leah Smith: It's fine.
Luke Marsden: Great.
Luke Marsden: And then we can get into this sort of rhythm of it of like getting it kicking off in the evenings.
Luke Marsden: Evening probably works better than early morning because you need to be there for the login step.
Luke Marsden: So you want to do it at the end of your day and then leave it on until midnight or whatever.
Luke Marsden: And then it just looks to LinkedIn like you're working hard that day.
Luke Marsden: Of course, you are working hard anyway, but you know what I mean?
Luke Marsden: Even harder.
Tony: Yeah.
Tony: And I guess obviously Lear is keen to work on the website as well.
Tony: Get that.
Luke Marsden: I'll get that fixed asap.
Luke Marsden: Yeah, I'll be Back online a bit later.
Luke Marsden: Yes.
Tony: Obviously the Mini Jack and Jill stuff, if there's a way to get anywhere with that, I guess.
Tony: Yeah, yeah, yeah, yeah, yeah.
Tony: Because I think we're keen to.
Tony: We haven't launched the.
Tony: The brand or anything yet.
Tony: We're still keeping that under wraps at the moment, but at some point, probably the next month or so, it'd be great to start pushing it out there a little bit.
Luke Marsden: Definitely.
Luke Marsden: Agreed.
Luke Marsden: Yeah, no, I think that's a good.
Luke Marsden: That's a good goal.
Luke Marsden: Kind of by the end of August.
Luke Marsden: Yeah.
Luke Marsden: I'm away for a week in August, so let me just check something on that.
Leah Smith: Again anywhere.
Leah Smith: Nice.
Luke Marsden: Crete.
Tony: Oh, lovely.
Luke Marsden: So, yeah, should be good.
Luke Marsden: Going with one of Rose's school friends, parents.
Tony: Oh, nice.
Luke Marsden: They're actually.
Luke Marsden: Well, the dad is from Crete.
Luke Marsden: Oh, that'd be pretty nice.
Tony: Local knowledge.
Luke Marsden: Yeah.
Luke Marsden: So I won't be around the week of 17th to 21st, but I am back the week after.
Tony: Okay.
Luke Marsden: So kind of by the end of August.
Luke Marsden: Let's aim to end of August, start of September.
Luke Marsden: But that would be cool, actually, to launch it like when everyone's back back at work after holidays probably, I think start of September.
Luke Marsden: Does that sound reasonable?
Luke Marsden: Yeah.
Luke Marsden: Okay.
Luke Marsden: Awesome.
Tony: Yeah, the agent looks great.
Tony: I guess it's just the only thing is working around the restrictions on LinkedIn.
Luke Marsden: 100%.
Luke Marsden: Yeah.
Luke Marsden: Yeah.
Luke Marsden: Thanks for navigating that with us.
Tony: Yeah.
Tony: Hopefully we've got a bit of a good solution where we can just share the project and.
Tony: Yeah, let's.
Tony: Let's see.
Luke Marsden: Yeah, good stuff.
Luke Marsden: And like I said, happy to use my account as well as needed.
Leah Smith: Sorry, last question.
Leah Smith: You might have covered it, I may have forgotten, but when, you know, you said you can leave it to run until midnight and then it will stop.
Leah Smith: Do you have to say to the agent, like stop working at midnight kind of thing?
Leah Smith: So then it will log off.
Luke Marsden: I will work on that.
Luke Marsden: Yeah.
Luke Marsden: We might want to enforce that both with a sort of soft and hard limit and.
Luke Marsden: Yeah, should be good.
Tony: So I just draw a guy.
Luke Marsden: Oh.
Luke Marsden: Oh, bloody hell, it's working.
Luke Marsden: I shouldn't sound so surprised.
Luke Marsden: That's great.
Luke Marsden: Awesome.
Tony: So if I do to collaborate, so if I click here, There's one in there.
Luke Marsden: So live.
Tony: So, yeah, I guess that InMail thing could.
Tony: Could work then that if it does draft an InMail, it could just send it to me and say, tony, I've just added Nishan to the.
Tony: To the pipeline, sending this in mail.
Luke Marsden: What we could do is put a bit of UI in the Find OS app that gives you all of those easy copy paste buttons and slacks.
Luke Marsden: You when it's finished and just go in, copy paste all of these, please.
Luke Marsden: Or we can do the.
Luke Marsden: Send the same draft to everyone.
Luke Marsden: Same template to everyone would be easier, but I guess it depends on numbers.
Luke Marsden: We could try both.
Luke Marsden: I don't know.
Luke Marsden: Oh, there's one more thing, Tony.
Luke Marsden: Did you see an email from me about the billing?
Tony: Oh, sorry, yeah, I did, yeah.
Tony: Let me send that to.
Tony: Do you want to send that to accounts?
Luke Marsden: It was more.
Luke Marsden: Yeah.
Luke Marsden: I was asking if it'd be possible to switch over onto a credit card if you're open to it.
Luke Marsden: Just for that.
Luke Marsden: I think it's a 499amonth for the platform.
Tony: It should be fine.
Tony: Account to make sure that that's all fine.
Luke Marsden: Yeah.
Luke Marsden: If you want to keep doing invoices, that's also totally fine.
Luke Marsden: I just wanted to make it easier.
Tony: Okay.
Luke Marsden: But, yeah, just give me a shout about that.
Luke Marsden: Lovely.
Tony: But this is promising, isn't it?
Luke Marsden: Yeah.
Tony: Me being logged in and it's just going to keep pushing them into my pipeline.
Tony: Amazing.
Luke Marsden: Awesome.
Leah Smith: Sorry, Luke, how long do you think.
Leah Smith: How long is a piece of string, but how long roughly does it take for the agent to put them into the.
Leah Smith: Because obviously one's come through already.
Leah Smith: Will they just keep coming through quite quickly, do you think, or is it like a slow burner?
Luke Marsden: It's a really good question.
Luke Marsden: It's looking like more of a slow burner just because the agent is a little bit slow at clicking around.
Leah Smith: Okay.
Luke Marsden: And a lot of them are getting filtered out because they've already been sourced by someone else or they're not a good fit.
Luke Marsden: But it's also worth looking.
Luke Marsden: It might actually just very quickly be worth looking at.
Luke Marsden: Go back to the.
Luke Marsden: To find OS and ask the agent or even just look with our eyes.
Luke Marsden: How many is it working through?
Luke Marsden: Was it like 3.1 million or was it 150, you know?
Leah Smith: Yeah, 6.8 K. Okay.
Leah Smith: Yeah.
Luke Marsden: So we might want to put some reasonable limit on that and say stop after you found how many.
Leah Smith: Or keep going.
Luke Marsden: Or keep going forever.
Tony: I just think leave it personally.
Tony: Just let us do his thing in the background.
Luke Marsden: I would put a time limit on it.
Tony: Would you?
Tony: Do you need it?
Tony: Yeah.
Leah Smith: No, do you mean Luke can say.
Leah Smith: And so it doesn't run throughout the night and make it look suspicious kind of thing?
Luke Marsden: That's what I'm thinking.
Luke Marsden: Yeah.
Tony: Yeah.
Tony: Right.
Tony: Okay.
Luke Marsden: I think there's some sort of middle ground where, I mean, we kick this one off at like half past three, so run it until half eleven or something, but don't have them working through the night for multiple 24.
Luke Marsden: Seven periods.
Tony: Yeah, sorry.
Luke Marsden: Yeah, no, no, it's fine.
Luke Marsden: I hear where you're coming from, like, more is better up to a point.
Luke Marsden: There probably comes a point where if I drop like a project with a thousand people on one of your colleagues, then they'll be like, I can't do all of that.
Luke Marsden: I don't know.
Tony: Did you get the AI to.
Luke Marsden: Yeah.
Tony: Filter it down again?
Luke Marsden: Yeah, yeah, yeah, yeah.
Luke Marsden: Well, we'll get, we'll, we'll get there.
Luke Marsden: Good stuff, right?
Tony: Amazing.
Luke Marsden: Yeah.
Tony: That's really, really good.
Luke Marsden: Thank you so much.
Luke Marsden: Yeah, no, I'm, I'm happy with that.
Luke Marsden: We're getting there.
Luke Marsden: Yeah.
Luke Marsden: All right, Cheers.
Leah Smith: Thank you.
Luke Marsden: Have a good evening.
Tony: Bye.
Luke Marsden: Bye.

