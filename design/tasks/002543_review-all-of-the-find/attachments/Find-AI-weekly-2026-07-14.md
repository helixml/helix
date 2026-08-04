# Find-AI weekly project meeting

- **Date:** Tue, 14 Jul 2026 13:15:00 UTC
- **Participants:** luke@helix.ml, chris@helix.ml, leah.smith@linuxrecruit.co.uk, tony.chapman@linuxrecruit.co.uk, luke@mlops.consulting
- **Source:** fireflies

## Summary

- **AI Recruiting Workflow:** Integrated Gmail, LinkedIn, Notion for automated candidate sourcing and outreach with two-factor authentication and keyword searches.

- **Candidate Contact Rules:** No contact if emailed in last 2 weeks or responded in last 2 months to reduce fatigue and improve engagement.

- **Human-AI Collaboration:** AI recommends "open to work" candidates for human phone calls; AI calls limited to simple queries for transparency and trust.

- **UI & Product Development:** Building a recruiter-controlled interface for AI messaging and candidate review, targeting completion by next Monday.

- **Data Accuracy & Integration:** AI cross-references Bullhorn to avoid duplicate outreach and prioritize candidates, improving recruitment efficiency.

- **Ethical AI Use:** AI communications transparently identified to preserve trust; AI supports but does not replace human interaction.

## Transcript

Leah Smith: One of those days, you know, when you just think is.
Leah Smith: Yeah.
Leah Smith: With the.
Leah Smith: The daughter being.
Leah Smith: Yeah, I'm gonna take her to the doctors at half three.
Leah Smith: I think she might have a chest infection.
Luke Marsden: Oh, okay.
Luke Marsden: Okay.
Leah Smith: She's.
Leah Smith: I think.
Leah Smith: I don't know if it was last week I spoke to you.
Leah Smith: Well, I did speak to you last week, but she wasn't well last week and it's just like kind of carried on.
Leah Smith: Yeah.
Leah Smith: You know, and you think, oh, they're getting better.
Leah Smith: And then I took her to the childminders this morning and they were like, yeah, she's just not herself.
Leah Smith: And they just don't like to keep them when they're not themselves, which is.
Leah Smith: Thing is, you don't want to keep.
Leah Smith: Keep them somewhere.
Leah Smith: But also it's like, I've got work to do, like.
Luke Marsden: Yeah, yeah, yeah.
Luke Marsden: So where is she now?
Luke Marsden: Is she.
Leah Smith: My dad's here.
Leah Smith: He's come around because he's retired, so he's come around and she's asleep, so.
Luke Marsden: Okay.
Tony: So.
Leah Smith: Yeah, she's looked after, but it's just.
Leah Smith: It's hard to try and focus when.
Luke Marsden: Yeah, yeah, yeah, no, of course.
Luke Marsden: Absolutely.
Luke Marsden: Yeah.
Luke Marsden: No, better safe than sorry with these things.
Luke Marsden: And the GPS will generally err on the side of caution.
Tony: Hey, Luke.
Luke Marsden: Hello.
Luke Marsden: You all right?
Tony: Yeah, not too bad.
Tony: Yourself?
Luke Marsden: Good, good.
Luke Marsden: Well, thank you.
Luke Marsden: I was just saying, excuse me for being sleeveless, it's just a bit warm in here.
Luke Marsden: It's not the most.
Luke Marsden: It's not the most professional look, but anyway.
Luke Marsden: Yeah, yeah, good, good.
Luke Marsden: So I've been testing out getting the agent to do the workflow we went through last week and it's got.
Luke Marsden: Well, great.
Luke Marsden: So I wanted to start maybe by showing that to you.
Tony: Recorded as well.
Tony: Sorry.
Luke Marsden: Yeah, yeah, it's on the fireflies.
Luke Marsden: Yeah, yeah, yeah.
Luke Marsden: So we should be able to do that.
Luke Marsden: Yeah.
Tony: The guys as well.
Luke Marsden: Yeah, definitely.
Luke Marsden: And this is just like a first pass because we can then kind of systematize it a bit, like put it in a kind of UI for you and link it up to Slack and things.
Luke Marsden: But we're just sort of testing the basics here, so.
Luke Marsden: Let me share my screen, actually.
Luke Marsden: Were there other things you wanted to cover before we jump into that?
Tony: Happy to jump into that initially.
Tony: Yeah, for sure.
Luke Marsden: Sure.
Tony: Leo is probably interested in the website and stuff like that as well, if there's any developments on that.
Tony: Yeah, may not be.
Tony: If you've been doing this.
Luke Marsden: Yeah, not really that's the update on that.
Luke Marsden: But you can remind me what you wanted adding there.
Luke Marsden: I think we Were talking about the having it actually reflect the roles that are tagged somehow in Bullhorn.
Luke Marsden: We're not quite there yet with the full horn integration.
Leah Smith: Yeah.
Leah Smith: I have logged a ticket with them this morning.
Leah Smith: So, yeah, hopefully they.
Tony: Yeah, the kind of Mini Jack and Jill was the bit, wasn't it?
Tony: How.
Tony: That's how that's maybe gonna look.
Luke Marsden: Yes.
Luke Marsden: Yeah, yeah, yeah, yeah.
Luke Marsden: So I think we'll get to that probably in a few days.
Luke Marsden: Okay, cool.
Luke Marsden: So let me share this.
Luke Marsden: So.
Luke Marsden: Make this a bit bigger.
Luke Marsden: So.
Luke Marsden: Yeah, so this is actually.
Luke Marsden: Let me just.
Luke Marsden: Full screen.
Luke Marsden: Can you still see that?
Luke Marsden: Oh, no, it's paused.
Luke Marsden: Because it's paused.
Luke Marsden: Yeah.
Luke Marsden: Resume.
Luke Marsden: Oh, come on.
Luke Marsden: Come on.
Luke Marsden: Zoom.
Luke Marsden: You can do it.
Luke Marsden: Want me to share my entire screen?
Luke Marsden: Okay, I will do that.
Luke Marsden: One second.
Luke Marsden: Okay.
Luke Marsden: Can you see that now?
Tony: Yeah.
Luke Marsden: Okay, cool.
Luke Marsden: So, yeah, I basically I fed everything in to Helix and.
Luke Marsden: And we're using Conduct as the example.
Luke Marsden: It was able to log into Gmail.
Luke Marsden: I gave it my login details.
Luke Marsden: So again, it will.
Luke Marsden: It prompted me for that.
Luke Marsden: It was able to log into LinkedIn and LinkedIn Recruiter.
Luke Marsden: It was able to scrape the details from notion.
Luke Marsden: It was also able to download the audio file from here and then it was able to transcribe the audio.
Luke Marsden: So it was able to slurp in everything we gave it, which is pretty cool.
Luke Marsden: And.
Luke Marsden: And yeah, the.
Luke Marsden: Is this where it logged in with me?
Luke Marsden: Tin Real account.
Luke Marsden: I did two factor auth on my phone to get it in.
Luke Marsden: It did a keyword search used open to work and then it pulled out these candidates.
Luke Marsden: Trying to recruit anyone as me.
Luke Marsden: But.
Luke Marsden: But.
Luke Marsden: To get to the keyword search option.
Luke Marsden: That's probably because.
Tony: Okay.
Luke Marsden: What we spoke about.
Tony: Cool.
Luke Marsden: Yeah.
Tony: What to do if they've already been contacted.
Luke Marsden: Yes.
Tony: Which is fine.
Tony: For.
Tony: For the next.
Tony: I'll just try it in there.
Tony: Contacted by someone in the last three months.
Tony: Don't contact anything like that.
Tony: I don't know.
Luke Marsden: Yeah.
Luke Marsden: And will that work?
Luke Marsden: Because we're.
Luke Marsden: Yeah.
Tony: Now on.
Luke Marsden: I gotta hold my hands up.
Luke Marsden: Feedback as well about the.
Tony: Yeah,.
Luke Marsden: Together.
Luke Marsden: That would be good.
Luke Marsden: Oh, one sec.
Tony: Yeah, it's not too bad, I think.
Leah Smith: Slideshow for some reason.
Leah Smith: Alex put another slideshow all pulling through on there.
Tony: Right.
Leah Smith: So what I've done is just edited that slideshow, the new slides, because I have no idea why one would be working and why is it one isn't?
Tony: This is the thing I think they keep saying, oh, it's the TV or it's the connection, so.
Tony: Well, no, it's not because it's not Even playing at all.
Leah Smith: Be working in one slideshow, it like.
Leah Smith: It doesn't make sense.
Tony: Other slideshow it's obviously been.
Tony: I don't know.
Leah Smith: I think they've done an update there.
Leah Smith: And.
Leah Smith: And just something.
Leah Smith: I don't know if something's not working or they've done.
Leah Smith: Yeah.
Tony: Project name fd.
Tony: That's pretty mad.
Tony: Be watching a robo job.
Tony: You're right.
Luke Marsden: Yeah.
Luke Marsden: Good.
Tony: Created this project called Conduct AI FT Live Demo.
Tony: It's called it.
Tony: Okay, let's work this.
Tony: Where we got to.
Luke Marsden: And that's all good, right?
Tony: Yeah, yeah, perfect.
Tony: And now it's searching within the project.
Tony: Which is.
Tony: Which is good.
Tony: Goes through the candidates.
Tony: Here we go.
Luke Marsden: It's kind of amazing to watch it working.
Tony: There you go.
Luke Marsden: This is.
Tony: This is one.
Tony: So if you look most recent activity, you can see James Yarrow has sent him an email.
Luke Marsden: Oh,.
Tony: So that message.
Tony: They've written a message, in a way.
Luke Marsden: Yep.
Luke Marsden: The agents.
Luke Marsden: It's not gonna.
Luke Marsden: I told him not to send.
Luke Marsden: So it's not gonna.
Tony: Yeah, that's cool.
Tony: Is that one.
Tony: Is that one on the.
Tony: On the list of.
Luke Marsden: I can ask.
Luke Marsden: So I can say.
Luke Marsden: Did you base that in mail off one of the samples from Tony's email?
Luke Marsden: And then if you hit enter twice, it interrupts what it's currently doing.
Tony: Ah, good little hack that I've never noticed.
Luke Marsden: Yeah,.
Tony: I do a.
Luke Marsden: Yeah.
Tony: People think that might be a sign.
Tony: If it's.
Tony: I always say, even if it's grammatically.
Luke Marsden: AI tells perfect.
Luke Marsden: So the nice thing is.
Luke Marsden: Spec then for how we build out the full bot.
Luke Marsden: The thing it was about.
Luke Marsden: We also need to.
Luke Marsden: Don't contact people who have already been contacted by someone else.
Tony: Always the term probably is in mail.
Tony: It was on the screen a minute ago.
Tony: James.
Leah Smith: In mail opened or in mail responded to.
Tony: Yeah, if someone sent them an email, that's fine.
Tony: If they responded, then that's probably not fine.
Tony: If that makes sense.
Tony: But we probably need two.
Tony: Two different rules.
Tony: One, if we send them an email in the last two weeks or last week, then send them another one, they replied within the last two months, then send them another one if that makes sense.
Luke Marsden: Like that.
Tony: Perfect.
Luke Marsden: And I feel like my job of typing what you were saying is also being replaced by the AI because we're going to feed the transcript into.
Luke Marsden: Into.
Luke Marsden: Into the agent at the end of the call as well.
Luke Marsden: But.
Luke Marsden: Yeah.
Luke Marsden: So, yeah, I think fundamentally we can get this thing actually running fairly quickly now.
Tony: Can I ask why Maximus over the others.
Tony: What was the.
Tony: Yeah, so it scrolled down and Clicked on Maximus.
Luke Marsden: Yeah.
Luke Marsden: Because when you were showing me you open up the profile and then use the left and right buttons, didn't you?
Tony: Yeah, I did it slightly differently.
Tony: I opened up the first profile and then clicked through.
Luke Marsden: Yeah.
Tony: Which is fine.
Tony: Either way it's fine.
Tony: And it also went straight to compose message rather than looking at the profile first.
Luke Marsden: Yeah.
Tony: I don't know how it's ranked them.
Tony: Would it, would it type you an answer or what?
Luke Marsden: Would it what when I asked the question.
Luke Marsden: Yeah, yeah it will.
Tony: Yeah, yeah.
Luke Marsden: Having a little think might come back.
Tony: With an amazing response.
Tony: Tony does it wrong.
Luke Marsden: He's an idiot.
Luke Marsden: Well, I told it you'd be on the call so hopefully it's not going to insult you.
Leah Smith: It talks like a human as well.
Leah Smith: Fair.
Luke Marsden: Yeah.
Tony: Where is he reading that though?
Luke Marsden: The search headline?
Luke Marsden: I think it's this thing but that, that's only.
Tony: Oh yeah, sorry.
Tony: Agency workflows.
Tony: Yeah, yeah, cool.
Tony: Okay, fair enough.
Tony: So put him had the PhD.
Tony: Okay fine.
Tony: So, so.
Luke Marsden: And you see the whole profile each time.
Tony: That, that might.
Tony: I mean this point is probably quicker looking at that.
Luke Marsden: Going at least looking at the whole profile is better.
Tony: Yeah.
Tony: Is that, is there like a scoring system there that it will be using do you think?
Tony: Or is it.
Luke Marsden: It's just made one up.
Luke Marsden: Yeah, but it made it up based on the very clear spec in here.
Tony: Yeah.
Luke Marsden: Which I understood to be all about AI engineering but not like at too low level.
Luke Marsden: Yeah.
Luke Marsden: Anyway, yeah.
Luke Marsden: Okay, so now it's saying let, let me do it Tony's way.
Luke Marsden: Yes.
Tony: That top where it says 1 of 1398.
Tony: See that top Right.
Luke Marsden: This is exactly why reading the profile matters.
Tony: Oh yeah, yeah.
Luke Marsden: Skip on two counts.
Luke Marsden: Okay.
Tony: There you go.
Luke Marsden: Oh and look.
Luke Marsden: Yeah, you can already see that Alan, Alex and Ethan have.
Tony: He looks really good.
Luke Marsden: But too soon to touch.
Luke Marsden: Yeah, but what are you thinking?
Luke Marsden: Are you thinking we should.
Tony: The guys have probably got in process at the moment.
Luke Marsden: Yeah.
Leah Smith: When the AI sends is it coming on behalf of you?
Tony: Yeah, it's logged in.
Luke Marsden: Well it would actually come from me at the moment because I'm logged in with me.
Luke Marsden: Yeah, yeah.
Luke Marsden: Because.
Luke Marsden: Yeah, yeah, exactly, exactly.
Luke Marsden: So I'm just testing do any mess ups that I take responsibility for myself.
Luke Marsden: But yeah, we.
Luke Marsden: We can have it running as whoever you like because it will be whoever logs in.
Luke Marsden: So I guess whoever's driving it.
Luke Marsden: So he passed dedupe.
Luke Marsden: That's good.
Luke Marsden: This time based on Tony's InMail1 and their voice, no AI tells.
Luke Marsden: What do you think of that?
Tony: Yeah, looks perfect.
Tony: Can you scroll down in that.
Luke Marsden: Yeah, yeah.
Luke Marsden: All right.
Tony: So we should add this person to say to the pipeline as well.
Tony: So if we like someone, we can save it to the pipeline.
Luke Marsden: Cool.
Luke Marsden: And remember, the whole thing we're doing here is building a plan that will then be reused by the agent that actually does this at scale.
Luke Marsden: So this is great.
Luke Marsden: Cool.
Tony: API for this.
Tony: But I can send you a ball.
Tony: Get the agent to cross reference with Bullhorn to see if they're on there as well.
Luke Marsden: Yeah.
Tony: They're open to opportunities.
Tony: Those open to opportunities.
Tony: If they're on there, we can make a note of that somewhere because we can actually give them a call.
Luke Marsden: Yeah.
Tony: Rather than having to wait for an InMail and then the response.
Tony: And also you get a finite number of inmails, so we need to.
Tony: If we got someone's contact details already, that should be our first port call.
Luke Marsden: Kind of thing to actually pick up the phone to them.
Tony: Yes.
Luke Marsden: Yeah.
Tony: So the guys pick up the phone and say, oh, you've come up on a search, you're open to work.
Tony: If someone's clicked open to work, they're typically.
Luke Marsden: They want to be happy to receive a call.
Luke Marsden: Yeah.
Luke Marsden: So I'd say this.
Luke Marsden: This suggests that we also need a way to give the team.
Luke Marsden: Yeah.
Luke Marsden: List of phone calls we suggest they make.
Luke Marsden: Right.
Luke Marsden: We still want a phone call from a human.
Luke Marsden: There's no way we're going to get an AI to.
Tony: Yeah.
Luke Marsden: Come across as a human on the phone.
Tony: Not yet.
Luke Marsden: Like, not yet.
Luke Marsden: It will happen.
Luke Marsden: It'll happen.
Luke Marsden: And it's probably fairly good already, but I just feel like that crosses a line for me.
Tony: Oh, totally.
Tony: That's.
Tony: Yeah.
Luke Marsden: Still too shit to be fair.
Tony: I got a AI phone call the other day.
Tony: It was clearly a robot, but actually I was like.
Tony: It was interesting.
Luke Marsden: Yeah.
Tony: And I was able to ask the.
Tony: The bot a few questions and it answered them for me.
Tony: Really factually.
Luke Marsden: Yeah.
Tony: Reggae sales.
Tony: He answered them for me.
Tony: I was like, okay.
Tony: And would you like me to set up a meeting with an actual person?
Tony: Yeah, go on.
Tony: So I found it quite useful firing questions at this without, you know, having to make small talk or, you know, go through the little.
Tony: So it's probably better than a normal cold call.
Luke Marsden: That's interesting.
Luke Marsden: And it was.
Leah Smith: It.
Luke Marsden: Yeah.
Luke Marsden: You liked it because it was upfront about the fact it was a bot.
Tony: Yeah.
Tony: It didn't try.
Tony: I mean, it sounded.
Tony: Tried to sound like a human, but it didn't.
Tony: I think it might have said, you know, I'm not a human or something like that.
Tony: So it was Obvious it wasn't trying to hide the fact that.
Tony: Yeah, it's about new ats.
Tony: So like.
Tony: Like a bullhorn.
Luke Marsden: Yeah.
Tony: So yeah, I asked it a few questions.
Tony: It gave me a few good answers that probably a human wouldn't have been able to.
Luke Marsden: Yeah.
Tony: As to the point and factual.
Tony: Yeah, it needs to be a human.
Luke Marsden: Yeah, yeah, yeah, yeah, yeah.
Luke Marsden: So it's just updating the plan with that idea and I'll figure out how to get a list of calls.
Luke Marsden: I guess we'll call it Find os.
Luke Marsden: Probably something like having a list in the.
Luke Marsden: In the web interface that's going to be built and maybe surfacing them every morning on Slack or something.
Tony: Yeah, sounds good.
Luke Marsden: Would that be the best thing or like after morning and after lunch or.
Tony: Well, yeah, yeah, morning would be great.
Tony: But there's also the guys have saved searches set up as well that they go through manually and message people.
Tony: So save searches are people who've clicked open to opportunities.
Tony: This would be another use case for.
Tony: For the.
Tony: For the agent which would be going through those and cross referencing bullhorn, see which ones details for clicked opens opportunities and come up on our alert.
Tony: We get a list saying these people, we've got their details, give them a call type thing.
Luke Marsden: Yeah, nice.
Luke Marsden: Okay.
Tony: That'd be really useful as well.
Luke Marsden: Yeah, yeah, yeah, awesome.
Luke Marsden: Great.
Luke Marsden: I mean I think I've got enough here to crack on then like this, this.
Luke Marsden: I'm excited about this because.
Tony: Would it be useful to have my LinkedIn and start messaging people like having it message people or not for now.
Luke Marsden: Or give me a few days?
Luke Marsden: Because what I'm trying to.
Luke Marsden: What I want to do is basically wrap this up in a bit of UI that I give to you to drive and test.
Luke Marsden: So I'd like to do that and then, and then it's almost like.
Luke Marsden: Well, once, once that's done then we can do that together.
Luke Marsden: But you can drive it.
Luke Marsden: Yeah.
Luke Marsden: As you're with your login.
Luke Marsden: So that's probably.
Luke Marsden: Probably how to do it.
Tony: Okay, cool.
Luke Marsden: That works.
Luke Marsden: So yeah, I mean realistically aim to do that in the next meeting on Monday, next week.
Luke Marsden: That would work for me.
Tony: Okay, sounds good.
Luke Marsden: Yeah.
Luke Marsden: Cool.
Luke Marsden: And I'll try and make a bit of progress on Jack and Jill as well.
Luke Marsden: And what.
Luke Marsden: What were the website bits?
Luke Marsden: Remind me.
Luke Marsden: I know you asked me to change the website a bit.
Leah Smith: It was remove the AI powered candidate.
Luke Marsden: Yep, that's done.
Leah Smith: Yeah.
Leah Smith: And I think Tony did we jobs carousel at the moment from the homepage and just keeping those jobs on the.
Tony: Yeah, I mean it doesn't really matter.
Tony: But if you click View more View right anywhere.
Tony: But we're not really pushing this out in a way yet, are we?
Tony: Really so kind of this massive problem.
Luke Marsden: But you.
Luke Marsden: I also needed to plug the emails in.
Luke Marsden: I remember.
Tony: Oh, did you?
Tony: Okay, so if we put in.
Luke Marsden: Emails don't actually work yet, I think.
Tony: No.
Luke Marsden: Okay, so I'll fix that.
Tony: Okay.
Luke Marsden: Wicked, Tony.
Luke Marsden: Thanks, leo.

