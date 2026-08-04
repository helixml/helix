# Find-AI kickoff!

- **Date:** Mon, 08 Jun 2026 14:00:00 UTC
- **Participants:** luke@helix.ml, chris@helix.ml, leah.smith@linuxrecruit.co.uk, tony.chapman@linuxrecruit.co.uk, luke@mlops.consulting
- **Source:** fireflies

## Summary

- **Agent for Business Development:** Automates outreach and data handling to boost efficiency in candidate sourcing and lead generation.  
- **Job Advert Strategy:** Focus on broader industry contacts instead of job advert chasing for higher engagement rates.  
- **Integration with Bullhorn:** Enhances candidate matching and data quality by cross-referencing LinkedIn and updating profiles regularly.  
- **Website Launch Priority:** New Find AI website must precede agent deployment for branding alignment and effective market entry.  
- **Human Involvement in Matching:** A controlled matching platform will use a two-agent model while maintaining consultant oversight for quality.  
- **Weekly Meetings Established:** Regular check-ins to monitor progress on the website and the internal agent development.

## Transcript

Tony: Find out in a minute.
Tony: That's the note taker thing.
Tony: How's your weekend?
Leah Smith: It was good.
Leah Smith: I went out on Saturday to this, like, R B in Southampton.
Leah Smith: So I'm feeling it today still, you know, but.
Tony: Is that your first night out?
Tony: Is it since.
Leah Smith: No, it's like the second one, but yeah.
Leah Smith: It's not fun having a child and then all you want to do is like, sleep and like, feel better and you can't.
Leah Smith: It's really not.
Luke Marsden: Not great.
Tony: That feels that.
Tony: That is life now, unfortunately for you.
Leah Smith: Know, and you're like, I just want to have a nap and just try better.
Leah Smith: But you can't.
Tony: And.
Leah Smith: Yeah.
Tony: In the day, though, could you have a nap at the same time?
Leah Smith: Yeah, she doesn't nap for long.
Leah Smith: She only naps for like half an hour, 40 minutes.
Tony: Oh, really?
Tony: Really?
Leah Smith: Yeah.
Leah Smith: I don't know why it's just cat.
Tony: Naps, but it's a power nap.
Leah Smith: Yeah.
Leah Smith: So, yeah, yesterday was a struggle, but yeah, today, Today feeling better, but yeah, just takes a day or so, doesn't it, when you get older.
Tony: Yeah, it really does.
Tony: I went to watch the boxing.
Tony: The bck.
Leah Smith: Oh, yeah.
Tony: Yeah, it was really good, actually.
Tony: Who won?
Leah Smith: Did he win?
Tony: He won.
Tony: Yeah.
Tony: Yeah, that's it.
Tony: Are you all.
Tony: Luke?
Luke Marsden: Hey, how's it going?
Tony: Yeah, good, thanks.
Leah Smith: How are you?
Luke Marsden: Yeah, very well, thank you.
Luke Marsden: Excited to have you on board.
Luke Marsden: Yay.
Tony: Yeah, likewise.
Luke Marsden: Yeah.
Luke Marsden: Good stuff.
Luke Marsden: Yeah.
Luke Marsden: So, I mean, I think in this conversation it'd be most useful to kind of go into the next level of detail in terms of what you're thinking.
Luke Marsden: If there's anything that you can show us or just talk through in terms of how you would like the agents to work, I think on both sides, both the user facing Jack and Jill stuff as well as the internal acceleration stuff we talked about.
Luke Marsden: That would be great.
Luke Marsden: It would also be helpful to understand a bit more about what your current database looks like.
Tony: Yeah.
Luke Marsden: How we might be able to interact with it.
Tony: Okay.
Tony: Where would you like us to start?
Tony: Do you want us to start with the internal agent or do you want us to start with the.
Tony: The website or what's.
Luke Marsden: Whatever's front of mind for you.
Tony: We can start with the agent.
Tony: It might be easy to talk you through, like general workflow and you can what.
Tony: What is possible and what.
Tony: What's not?
Tony: I guess with the.
Tony: With the agent.
Tony: So I was just trying to log into my email, but it's locked me out.
Tony: Hold on.
Tony: So, yeah, I guess I can kind of see the.
Tony: The internal agent working in.
Tony: In Two ways for us really.
Tony: So one is obviously new business development.
Tony: So obviously as a recruitment business we pick up, pick up jobs with companies and then we, we find candidates for those jobs.
Tony: Obviously we have a lot of clients who come to us, you know, regularly with volume based work.
Tony: We're always looking to pick up I suppose like any looking to, to increase the number of companies we're working with and the profile of those companies and all that kind of stuff.
Tony: So yeah, new business development.
Tony: There's probably quite a few kind of use cases I was thinking over the weekend what, how we actually do it manually at the moment and see a few different ways that we, we generate business.
Tony: One way that I think would be really easy for, for an agency would be to job scrape like advert chase.
Tony: So job scrape adverts that are online go and find the people at those companies who might be the hiring managers or the internal TA teams and send personalized messages to them either via LinkedIn or via email.
Tony: Yeah, that's probably the simplest, but that's the, the advert chasing stuff for us is probably the lowest value kind of development that we do because obviously our company's already decided to post an advert up and we're, we're, you know, a little bit behind on, on that job going out there kind of thing.
Tony: There's probably some general just finding people in the, in the industry on LinkedIn and just sending them messages, automated messages just by scraping companies who, who hire for that skill set, adding managers and sending messages to them in that way.
Tony: Also a lot of what we do is we, we will pick up kind of market intel from, from candidates or maybe they might mention they're interviewing at certain companies and we would, then there'd be a process that would follow from getting that lead, as we'd call it, to an introduction.
Tony: Say if we're able to pick up leads from people that we're speaking to and then just feed them into the agent and the agent does that kind of work behind the scenes.
Tony: We just say Tesco hiring for a data engineer or whatever and then it kind of goes and does its thing.
Tony: That's kind of what I've got in mind.
Tony: Obviously, you know, fairly conscious that we don't want to make it look like it's AI outreach because I think there's probably a bit of a stigma of that as well.
Luke Marsden: Yep, yep.
Luke Marsden: Yeah.
Tony: So I mean you might have some ideas on, on how best to approach it.
Luke Marsden: The, the way I found it works best is if you treat it like an actual agent.
Luke Marsden: Human collaboration between literally like you and the agent, where the agent will ask you to log in to LinkedIn and you'll do two factor authentic at some point it will then remember that login for a while so you don't have to do that all the time.
Luke Marsden: But, but then the agent kind of collaborates with you on it, finds an opportunity and then it gets you to draft the first message.
Tony: Okay.
Luke Marsden: So it actually sounds like it comes from you because it really does.
Tony: Yeah.
Luke Marsden: But then, and then you do a few more of them like that and then it can kind of pick up the pattern and then it will start suggesting messages and so on.
Luke Marsden: And then from that point it's up to you whether you leave it like going, whether you think the messages are good enough that it's coming up with or if you want to stay in the loop.
Luke Marsden: But I've had some pretty good success with that just in booking my own meetings.
Tony: Okay, that sounds a good idea.
Tony: Also, we've probably got, we've got hundreds of example emails and messaging that we've sent out previously.
Tony: Is there a way to feed that in up front so it can kind of learn from those?
Luke Marsden: Pretty much.
Luke Marsden: I mean it would.
Luke Marsden: It, you can just ask it to look through the, the previous messages you sent out on a given LinkedIn account.
Tony: Yeah.
Luke Marsden: How many people are doing this on a day to day basis?
Luke Marsden: Is it, is it you, Tony, or.
Tony: Is it like it's mostly myself, but the team are also doing it sporadically.
Tony: We're kind of keeping an eye out for opportunities really.
Tony: So we're always having these kind of conversations.
Tony: No one whose dedicated job it is to do.
Luke Marsden: Yeah.
Tony: So there's not a single person who's sat there, you know, contacting people all day.
Tony: It's, it's just sporadic as and when it happens.
Luke Marsden: So it's mostly about giving you a mechanical suit.
Tony: Yeah, yeah, exactly.
Tony: Yeah.
Tony: Creating another one of me would be.
Luke Marsden: Yeah, yeah, yeah.
Tony: Not sure if that's something for everybody else, but.
Luke Marsden: Yeah.
Tony: And obviously on, on the candidate side, probably the value for us typically, especially when the market is, is quite busy and there's a lot of jobs and not many candidates that the value really for us is sourcing those good people.
Tony: And obviously we, we've got a team of people who are, who are doing that all day every day as sourcing candidates.
Tony: So that, that's probably where the most value is going to be for us with the, with the agents.
Tony: Because a lot of, although there's obviously a lot of skill involved once you've got A candidate kind of on the, on the hook.
Tony: I guess a lot of the initial outreach is a little bit of a numbers game to an extent because you know, on LinkedIn we get like a 30% response rate so there's no point spending hours and hours finding the perfect person to message because the likelihood of them getting back to you is still quite slim.
Luke Marsden: Yeah.
Tony: So yeah, it's not like a spray and pray approach, but you know, know we do send a lot of messages compared to the amount of people we actually end up speaking to, if that makes sense.
Tony: We kind of.
Tony: So if we're on the fence about whether someone's great or, or not great, we'll send them a message anyway and then we'll triage your responses.
Tony: Who, who might be the best fits and kind of qualify from there.
Tony: So yeah, def.
Tony: Definitely LinkedIn messaging based on job descriptions or notes from, from meetings.
Tony: We record all of our briefings on a, on a, a voice note and we put it into notebook.
Tony: I'm not sure if you've seen that.
Tony: And we can push out like notes and follow ups and stuff like that from them.
Tony: So we've got all the voice recordings from, from all of our, our briefings.
Tony: So lots of notes and stuff that we can feed in database.
Tony: So we use a CRM called Bullhorn.
Luke Marsden: Pool of what, sorry?
Tony: Bullhorn.
Tony: B U L H. Okay.
Tony: So it's, it's the most widely used recruitment CRM and I, I can give you a quick tour of that if you, if you want to have a quick look.
Luke Marsden: I mean.
Luke Marsden: Yeah, it would be useful to, to just kind of see the different objects in the system.
Tony: Yeah.
Luke Marsden: And on the recruitment, on the candidate sourcing side.
Tony: Yeah.
Luke Marsden: How many people are currently doing that work day in, day out?
Tony: So we've got six at the moment doing that.
Luke Marsden: Cool.
Luke Marsden: Yeah.
Tony: Still relatively small teams.
Tony: It's not.
Tony: Yeah.
Tony: The idea of the agents is to free them up to spend more time actually speaking to people.
Tony: I think to automate a lot of the, the kind of time consuming sourcing side of things and that'd be useful.
Tony: So this is Bullhorn.
Tony: Say you've got, you know, we keep all our vacancies here, candidates, managers, companies.
Luke Marsden: It looks like they have an API as well.
Luke Marsden: Which.
Tony: Yeah, so we've probably got the details for that actually already the API key.
Tony: So these are all of our jobs.
Tony: Let's just dive into one.
Tony: For instance, this is just one that Toby's working on.
Tony: So we've got all of our candidates into shortlists.
Tony: Then the idea is to move Them through to cv, sent then into use and placements kind of thing.
Tony: Each candidate has its own record.
Luke Marsden: How much of the process do you run in terms of the interviews versus well, actually I guess it's mostly just introduction to hiring manager and then take it from there.
Luke Marsden: Right?
Tony: Yeah, we'd manage the candidate from.
Tony: From there on.
Tony: So we'd do all the scheduling, present all the feedback, but obviously the actual interviews are done by.
Luke Marsden: Yeah, of course.
Luke Marsden: Yeah, yeah, yeah.
Luke Marsden: Oh, okay.
Luke Marsden: But you handle the scheduling.
Tony: Yeah, we'll do everything from start to finish.
Tony: For most times there's a couple of.
Tony: Particularly a couple of work within the US Who.
Tony: The US seems to be a little bit different.
Tony: They like a bit more direct contact, but we will still have like the touch points for the candidates throughout.
Tony: We'll still, you know, kind of almost work with them to provide feedback and would manage offers, the offer process and all that kind of stuff.
Tony: But yeah, for most clients we do the whole scheduling.
Tony: So we'd send them qualified candidates, spend a bit of time with them, send the cv, get feedback interview, we'd schedule that into their calendars and whatnot, present interview feedback, the next interview and so on.
Tony: So yeah, cool.
Luke Marsden: Okay, great.
Tony: This is the candidate and this is.
Tony: This shows how kind of basic it is.
Tony: So I don't know, let's say we wanted someone with kubernetes and aws or is it so simple?
Tony: Simple Boolean search is what we do on here.
Tony: We can chuck in some additional criteria like location, all that kind of stuff, but it's not that.
Tony: It's only as good as the data we've put on the database.
Tony: A lot of the time the guys won't spend a lot of time.
Luke Marsden: Making.
Tony: Sure the location is correct and all that kind of stuff.
Tony: So let me search this.
Tony: So that would bring up.
Luke Marsden: I thought it was right, actually.
Luke Marsden: I was going to say learning how to spell Kubernetes as well.
Tony: I should know that one after.
Luke Marsden: No, I think you did it.
Tony: Yeah.121 canceling database.
Tony: There should be more than 300.
Tony: So I've obviously got some of the criteria.
Tony: Yeah, here we go.
Tony: 25,000 With.
Tony: So if we get, you know, if the core requirements are Kubernetes and AWS and Azure, there's no real way for us to sort through these either.
Tony: So we've got 24,000 there and it's almost like a bit of potluck.
Tony: Where do you start?
Tony: You can sort by when they were last added or something like that, but it's quite basic functionality in terms of searching the database.
Tony: Obviously we can, we can narrow this down quite a bit with different tools and tech and stuff like that.
Tony: It's not, it's not ideal also for Canada is if I go to do this the other way.
Tony: This is one.
Luke Marsden: Yeah.
Tony: 2015.
Tony: So these CVS are going to be.
Tony: And this is actually the actual date we moved to Bor.
Tony: So some of these will be even older than that.
Tony: So we've got cvs that are really, really old aren't up to date.
Tony: So if we're putting in some technology that exists after their CV was put on our database and they're obviously not going to come up on the search, even though they might be.
Tony: Might be quite good.
Tony: So if there's a way of somehow analyzing the data we got in here and cross referencing with LinkedIn and providing updated.
Luke Marsden: Yeah, refreshing that data, that would be.
Tony: Really useful for us as well.
Tony: But yeah, some way to kind of source these, you know, these guys in contact them would be super useful.
Tony: So we've got obviously, yeah, this, this database here, our CRM.
Tony: So we will have spoken to every candidate on here.
Tony: LinkedIn recruiter.
Tony: We use and we use one job board at the moment.
Tony: Job.
Tony: Job site.
Tony: That's mostly for the lower level kind of problems that we'll get in that maybe SAP in type stuff and support.
Luke Marsden: What was the name of the job board?
Luke Marsden: Sorry?
Tony: Job site or total Jobs.
Tony: Total Jobs.
Tony: Same boy.
Tony: So, so yeah, that's.
Tony: That's probably kind of it in, in a nutshell from a top level is that.
Luke Marsden: Yeah, that's really helpful.
Luke Marsden: I mean honestly, I think that's given me plenty to work with.
Luke Marsden: The.
Luke Marsden: My.
Luke Marsden: My instinct is to start getting some agents plugged in and into your slack and, and then actually just sort of start trying to do stuff and then it's right.
Luke Marsden: Yeah.
Luke Marsden: Rather than trying to do some sort of big design up front, let's just start.
Luke Marsden: Start with the lowest hanging fruit.
Tony: Yeah.
Luke Marsden: Is the lowest hanging fruit do you reckon like Tony finds new companies or is it the applicant sort of candidate side?
Tony: Possibly companies might be the first place, the best place to start.
Luke Marsden: Yeah.
Luke Marsden: Business.
Tony: Can we link it with Gmail as well?
Tony: So it's automatically sending stuff from my Gmail rather than from LinkedIn?
Tony: Yeah, okay.
Luke Marsden: Yeah, yeah, yeah, yeah.
Luke Marsden: And yeah, I mean the agents can do research using the browser in their own computer to figure out like contact details for people and checking websites and things like that.
Tony: So sorry, just before this has come through, the only thing about the new business stuff is obviously a lot of the business we're going, going at the moment is the AI machine learning type world and obviously we're building that.
Tony: The brand that goes alongside that.
Tony: So I'm just.
Tony: Now actually, is it worth waiting until we've got something there and then.
Luke Marsden: Yeah, we can put the website, we can, we can sequence it differently so we can do the website first.
Luke Marsden: Because otherwise you're going to be presenting yourself as Linux recruit.
Luke Marsden: You want to be presenting yourself as Find AI.
Tony: It might be worth doing the initial kind of BD campaign around the Find AI sort of things, which ties in nicely with, you know, the new brand and new website and that kind of stuff.
Luke Marsden: Okay, yeah, yeah.
Tony: Is that.
Leah Smith: Yeah, yeah, that makes sense.
Luke Marsden: And I guess you're not doing much outreach on that to find new companies yet anyway because of that same reason, right, Tony?
Tony: Exactly.
Tony: Yeah.
Tony: So it's complete blank canvas.
Tony: We don't.
Tony: Yeah, yeah, we've got some kind of marketing kind of BD assets that Leah's put together, which we've just kind of rebranded from the lynch crew side that'd be good for, for bd, but we haven't really pushed it at all.
Luke Marsden: Okay, cool.
Luke Marsden: Yeah, yeah, sounds good.
Luke Marsden: So we can do the initial pass on the website build first, get it hooked up so that it deploys automatically and then what I'll do there is I'll give you both the tools to make changes to that through an AI chat and then you can just.
Luke Marsden: It's the same way that we now update our own website.
Luke Marsden: We're just like, oh, I want this to look like that, or add a new case study for blah.
Luke Marsden: And then you just.
Luke Marsden: And then it just does it for you.
Luke Marsden: So that should be pretty accessible.
Tony: And is that just in Slack?
Luke Marsden: It can be in Slack or it can be in the Helix ui, as in the web interface.
Luke Marsden: So.
Luke Marsden: So both will work.
Luke Marsden: And the Helix web interface is useful if you want to get it to actually show you what it's doing, because it literally runs its own little desktop in the browser that has a browser inside it, if that makes sense.
Luke Marsden: While it's actually making changes to the website, you're looking at the dev environment.
Luke Marsden: It's almost like you're working with an engineer and they're sharing their screen.
Luke Marsden: We can try that first.
Luke Marsden: That sounds good.
Luke Marsden: It sounds like step one, new website now.
Luke Marsden: Then once that's up, step two, start doing Tony's mechanical suit, if you don't mind me calling it that.
Luke Marsden: And then.
Luke Marsden: So yeah, then.
Luke Marsden: And thanks for the info on the.
Luke Marsden: On the database.
Luke Marsden: That all looks fine.
Luke Marsden: I'll do like a sanity check of API access on that because probably we don't want the agents having to like log into the browser, into the web interface for the database every single time.
Luke Marsden: We probably want to do that as like an MCP server.
Luke Marsden: And then the other big area, of course, is the Jack and Jill stuff.
Luke Marsden: So do you want to talk me through that?
Luke Marsden: I now know what the database is.
Tony: Yeah,.
Luke Marsden: Yeah.
Luke Marsden: I don't know if.
Luke Marsden: Have you actually, you, have you tried Jack and Jill yourself or.
Tony: Oh, yeah, I did, just to get an idea.
Tony: Idea of what it.
Tony: And they wouldn't give me access because I was a agency recruiter, which is fine.
Tony: You know, I thought that might be the case.
Tony: You kind of like you set up a profile and upload a job description and then it verifies your account and it said, you know, apologies, we can't, we can't give you access or whatever.
Tony: Yeah, I actually think that it's, it's made an error because it keeps sending me candidates.
Tony: Yeah.
Tony: So I think it said no to me, but actually it hasn't told Jill that it shouldn't be sending, doing stuff for me.
Tony: So now it keeps, it keeps sending these, these emails with candidates on it.
Tony: So I can show you, actually, if.
Tony: I'm not sure if this is the way that we, we would want to do it, but this is what they're, they're doing.
Luke Marsden: Yeah.
Luke Marsden: Show me, show me what they're sending.
Luke Marsden: And then also just kind of brain dump at me what you want it to do and, and what you want your Jack and Jill to do.
Luke Marsden: And I don't know if we need to come up with different names for them.
Tony: Yeah.
Luke Marsden: Say Rosie and Jim maybe.
Luke Marsden: Fantastic.
Tony: Right.
Tony: So can you see that?
Tony: Yes.
Tony: It sent me, it just keeps sending me candidates on email.
Luke Marsden: Yeah.
Tony: Say three new candidates.
Tony: Four new candidates.
Tony: Expanding.
Tony: Yeah, they sent me one guy initially and then it said we're going to now expand this from our own database because I imagine their own database for the kind of person that I put in there is very, very small.
Tony: So it was just.
Tony: I just found an old advert for like an account manager with the job, but now it just keeps sending me these, these messages.
Tony: So it pulls them off.
Tony: LinkedIn sends me them like that.
Tony: This guy used to work here, funnily enough.
Tony: So, yeah, I, I don't know if there's more to it, if your account actually gets, gets authorized.
Tony: I'm not sure.
Tony: But yeah, that, that's what I got.
Tony: So just on email.
Tony: So obviously we've got our database and, and Keen to plug into that so people can search it.
Tony: But I don't know if we'll be able to just spin up every person on there as a searchable profile.
Tony: I don't know if it's, it's possibly worth.
Tony: Initially I was kind of thinking about this over the weekend, doing some kind of like campaign to them to say this is what we're, we're intending to do.
Tony: If you would like a profile on there, click here or upload new CV or whatever and then it automatically creates like a profile of them that's searchable.
Tony: That might be a good way to spin up some conversations with, you know, dormant candidates in a way, if that makes sense.
Luke Marsden: But.
Tony: Okay, I don't know if that's possible.
Tony: It's obviously quite a big example.
Tony: Over 100,000 on there.
Tony: I don't know if there's.
Luke Marsden: Yeah.
Luke Marsden: So is the question, do we do an outreach?
Luke Marsden: I guess you've already got tools for sending out email campaigns to your audience.
Luke Marsden: Right.
Luke Marsden: But you would maybe send them a link to register with Rosie and Jim.
Luke Marsden: I'm just going to call it that now, But it would be to register with Rosie.
Luke Marsden: Right.
Luke Marsden: And then Jim would be the second one which is dealing with the companies.
Luke Marsden: But yeah, I think I do like the idea of starting with a smaller group.
Tony: Okay.
Luke Marsden: Because that way we can test it with like, not everyone just in case, you know, like as we, as we iterate on it, we don't want to like mass mail the wrong thing to a large group.
Luke Marsden: We'll be very careful with that, obviously, anyway, as we go through.
Luke Marsden: But yeah, I think as soon as we've got.
Luke Marsden: It's going to take a while to build, obviously.
Luke Marsden: But as soon as we've got something that's ready for sort of beta testing, then we could find a few friendly candidates who are willing to sign up and give us their feedback of using it as well, I guess.
Luke Marsden: Do you have any companies ready to go on find AI?
Tony: Yeah, I mean, we've got a bunch of clients.
Tony: Yeah, we work with a lot of companies all the time, so we definitely have people who would be able to kind of would be to test that the, the rosy side of things as well.
Tony: So what, what I, what we've kind of got in mind is making it really, really easy for candidates to do this and say they ever upload their CV or they can create their profile.
Tony: Obviously the, the CV needs to be anonymized so name comes off, companies come off.
Tony: So then, you know, people can't just go on LinkedIn and find them themselves, if that makes sense.
Luke Marsden: Yeah, yeah.
Tony: Turning that CV into a snapshot of that person, you know, maybe years of experience where they're, you know, tools and technologies, all that kind of stuff.
Tony: Pulling off like the key highlights from their, from their profile.
Tony: Creating like a mini profile.
Tony: Yeah, Clients perspective, making it super easy for them as well.
Tony: So either they upload a job description or upload an advertisement and then it will create an advert on the site and push matches to them, if that makes sense.
Tony: Based on that, they can either put in a job description or they can put some kind of key criteria into a search panel and then it will push them out.
Tony: These anonymized profiles.
Tony: Click here if you want to Explore.
Tony: Speaking to candidate 235.
Tony: Yeah, we'll get an alert.
Tony: Candidate maybe gets an alert, say someone's interested, registered interested in your profile.
Luke Marsden: Yeah.
Tony: Speak to your find AI consultant or something like that.
Tony: So we're not losing the, the control of the placement.
Tony: We're just making it easy for our clients to plug in, have a look at people who might be interesting for them, saying they would be interested in speaking to them and then we kind of make it happen.
Tony: That's when the human element kind of comes in again.
Tony: Got it.
Luke Marsden: Yeah.
Tony: So different Jack and Jill, where you click on the, the link and it takes you to Jack and Jill and you have to do the whole process yourself.
Luke Marsden: Yeah.
Tony: Smaller fee, but you're kind of effectively doing it yourself.
Tony: Which, you know, I think most people don't really want to be doing that.
Tony: You know, tech people, they, you know, they just want to pass it over to an expert to kind of handle the process.
Tony: So that, that's kind of where we want to keep that human side of things rather than.
Luke Marsden: Yeah, no, definitely.
Luke Marsden: And I think it makes sense to keep the human in the loop and to massively the amount of work that your humans have to do.
Tony: Yeah, exactly, yeah.
Luke Marsden: Down to like being given like I guess like a candidate match or as in a possible match and then them taking it from there.
Luke Marsden: Okay, great.
Luke Marsden: I think that makes sense.
Luke Marsden: And I'm kind of just imagining like a bunch of these rosy and gyms like whirring away in the background.
Luke Marsden: There's triggers, right.
Luke Marsden: There's the event that someone uploads a new cv, there's an event that someone uploads a new job spec, and then there's sort of that ongoing matching process that happens in the background, which is just like a set of agents constantly working away at trying to fill a role, I guess is that right?
Tony: Yeah.
Luke Marsden: Yeah, okay.
Tony: That's right.
Tony: I, I don't know if there's some kind of scoring system like based on criteria match, but also maybe how recently they've put their, their profile up or how.
Luke Marsden: Yeah, well, what, what scoring system do you use informally?
Luke Marsden: Or like how do you, how do the humans do it?
Tony: Well, we, it would be a skills and kind of like personality match to the role.
Tony: So we'd have a.
Tony: Know requirements or criteria from, from the client side and we had to go away.
Tony: I mean the initial source.
Tony: This is why I know that an agent can easily do this because the initial kind of searching bit is just keyword matching.
Luke Marsden: Yeah.
Tony: To get the, the kind of right ballpark people in front of you and then it's obviously the conversations.
Tony: So yeah, it's just basic, you know, keyword matching initially.
Tony: Have they got the tools and tech and then it's okay, we'll speak to them and see if they've got the right kind of personality fit, the right kind of mindset, outlook or whatever.
Tony: The client's looking for the right skills for that role.
Tony: Obviously a lot, a lot of the time we know people already, so that's the one thing that AI might, the agents won't be able to do.
Tony: You know, it's okay.
Tony: I've got 10 people in my head for this role already.
Luke Marsden: Yeah.
Tony: If we can train the, the agents up against that point, then that'd be, you know, amazing.
Tony: But I'm thinking if, if we do a search on LinkedIn, we'll get a load of people that come up, say I don't know, a thousand.
Tony: But then it would say here's a thousand candidates.
Luke Marsden: This,.
Tony: 250 Of them are open to work, so you can click open to work button.
Tony: Another 200 people are more likely to respond.
Tony: They know what they base it on, whether it's.
Tony: They're kind of analyzing the people's interaction previously or whatever.
Tony: But they're.
Tony: There's a most likely more likely to respond category.
Luke Marsden: And that's the LinkedIn recruiter piece, right?
Tony: Yeah.
Luke Marsden: So that's kind of like one of the, one of the tools.
Luke Marsden: Do you pull Those candidates from LinkedIn Recruiter into your database or how does that.
Luke Marsden: Or do you only put them in a database once you've got a CV off them?
Luke Marsden: Or I mean, what's.
Tony: Yes, it's once we spoke to them really.
Tony: You can't, you can't accept.
Tony: You can export a links to LinkedIn profile.
Luke Marsden: You don't want to just fill your database full of like stale yeah, previously spoken to.
Tony: So if we spoke with someone on LinkedIn Recruiter, we'll always manage it through our CRM.
Tony: So we'd always move across from LinkedIn into our CRM and put it into the specific vacancy and that kind of stuff.
Tony: So normally, 99 times out of 100 would have a CV for that person.
Tony: But it might be.
Tony: We export their LinkedIn profile, they don't have a CV.
Luke Marsden: And in your.
Luke Marsden: In how you're imagining this working, do you still want that part to be human in the loop, as in the talking to them part, or do you want to try and automate on the phone conversation?
Tony: Sorry, do you mean on the phone or do you mean on LinkedIn?
Luke Marsden: Well, definitely on the phone, but I'm also wondering about, do you ever chat with these candidates on LinkedIn or do you go straight to trying to call them?
Tony: Yeah, so we'd reach out on LinkedIn.
Luke Marsden: Yeah.
Tony: There's probably a couple of different ways the guys would do it.
Tony: They'd either cross reference with Bullhorn.
Tony: Have we, you know, worked with this person before?
Tony: Do we have their number?
Tony: Just give them a call?
Luke Marsden: Yeah.
Tony: But most of the time it would be just messaging them through LinkedIn.
Tony: Some people send individual messages throughout the day.
Tony: Others would get a.
Tony: Put them into what's called a project on LinkedIn.
Tony: So essentially, it's like categorizing them into one area and then 50 in one go, and then see who responds to 50 people in the right ballpark for this job.
Tony: Basically put them into a kind of short list, essentially, and then message them all in one go.
Luke Marsden: Oh, so you can mass message through LinkedIn recruiter?
Luke Marsden: Yes, you can, yeah.
Luke Marsden: Yeah.
Tony: Cool.
Tony: You can.
Tony: Actually, what they.
Tony: What you can do on there now is you can personalized mass messages as well.
Tony: They've got like an AI thing which isn't very good.
Tony: We don't.
Tony: We don't really use it very often.
Tony: But yeah, you can personalize that.
Tony: We've put out the name and the company and stuff.
Luke Marsden: Yeah.
Luke Marsden: Cool.
Luke Marsden: Okay.
Luke Marsden: No, that makes sense.
Tony: Sorry.
Tony: Well, just before I forget, one other area I think could be good or could be kind of fruitful for agents is GitHub.
Tony: So everyone's got their email addresses or most people have their email addresses visible.
Tony: Yeah, Through a lot of candidates, kind of quickly on there.
Tony: The team will use that as a kind of sourcing tool.
Tony: They'll either go on there and find people and then cross reference with LinkedIn, or they will, you know, message via link.
Tony: GitHub.
Tony: I don't know if You've had that happen to you before and how you feel about people messaging you from there.
Luke Marsden: Or I've definitely used it the other way around when I want to get in touch with someone I know they're linked their GitHub account.
Luke Marsden: Then I clone one of their repos and their email addresses in the commit message.
Tony: Oh, okay.
Luke Marsden: That's a good trick.
Luke Marsden: So yeah, nothing stopping us getting the agents to do that.
Luke Marsden: They can certainly, they certainly know how to git clone things.
Luke Marsden: Yeah, the.
Luke Marsden: So yeah, I think that's a good one to throw in there and we should just be able to tell the agent to do that and it should do it.
Luke Marsden: So that should be fine.
Luke Marsden: Just coming back to that messaging, the kind of messaging candidates piece when trying to source candidates on LinkedIn.
Luke Marsden: Is that something you want Rosie and Jim to do?
Tony: No.
Tony: So I see Rosie and Jim is separate to the candidate sourcing sort of things.
Luke Marsden: Okay, yeah.
Luke Marsden: So just walk me through that then.
Tony: Why it's different.
Luke Marsden: Yeah, just the separation in your mind of those ideas.
Tony: The kind of the sourcing candidates I think is, is more just to help the consultants get through more of that kind of.
Luke Marsden: Got it.
Luke Marsden: So that's more on the internal side.
Tony: Exactly.
Tony: So that, that's.
Tony: I can see it that companies will go on to the, the company bit of Jim or Rosie or whatever, whichever will put their, you know, they're looking or whatever and put up a bunch of candles then pop up as a message saying they're interested in this person.
Tony: Obviously if we can then get that person interviewing with that company, great.
Tony: But if not, we've opened up a conversation.
Tony: They're looking for xyz.
Tony: Okay.
Tony: Now let's plug you into our consultants to find people job and also plug into the, the internal AI agent.
Tony: But we, we get new kind of jobs just called in or emailed in all the time.
Tony: Yeah, just from our kind of general outreach.
Tony: So I think it's more the kind of agents for sourcing is more we've got a role, we sign terms of a company and you know.
Luke Marsden: Yeah.
Tony: Yeah.
Luke Marsden: Okay.
Luke Marsden: Yeah, yeah, that makes sense.
Luke Marsden: Awesome.
Luke Marsden: I think that's a really good level of detail to get started with.
Luke Marsden: I'm sure, I'm sure we'll have more questions as we go through this.
Luke Marsden: But yeah, I think we can just start working on it now in terms of regular meetings.
Luke Marsden: Should we keep this time every week or like when's good for a regular check in?
Tony: Yeah.
Tony: So back to me.
Luke Marsden: Yeah, cool.
Luke Marsden: Chris, do you want to adjust it or I don't know this is your school run hour, isn't it, over in Oregon?
Chris Sterry: Yeah.
Chris Sterry: That's okay.
Chris Sterry: I was literally on my bike riding my kids to school while on the call.
Luke Marsden: So I'm fine with it.
Luke Marsden: As long as you don't mind doing that with your airplane asking.
Luke Marsden: Yeah.
Chris Sterry: And then I got back to my office and here I am.
Tony: Yeah, I know.
Chris Sterry: I was totally fine.
Luke Marsden: Chris, do you have anything to add or any comments or questions or anything?
Luke Marsden: How do you feel about this?
Chris Sterry: I feel pretty good.
Chris Sterry: I mean, I think.
Chris Sterry: I think the first step is just kind of, kind of capturing all of this and making sure we're on the.
Chris Sterry: The right page which we can send in a summary.
Tony: And then.
Chris Sterry: Sorry, I'm looking at my phone because you're on my phone as well and.
Chris Sterry: But you're also here and I don't know where I'm looking anymore.
Chris Sterry: Yeah, yeah.
Chris Sterry: So I think just make sure we're on the same page and then we can start cranking through.
Chris Sterry: So I don't really have any questions at the moment.
Luke Marsden: Okay, super.
Luke Marsden: Yeah, I, I think that gives me enough to, to work with.
Luke Marsden: I'm probably going to be doing most of this build myself, so you're dealing with me.
Tony: Okay.
Luke Marsden: And I'll.
Luke Marsden: I'll pull in some colleagues as, as and when needed.
Luke Marsden: Everyone else is.
Luke Marsden: Is busy on other customer projects at the moment.
Luke Marsden: Okay.
Luke Marsden: But that should be fun.
Luke Marsden: I'm looking forward to it.
Luke Marsden: I think sequencing basically is the new website first before we do anything on the find AI side.
Luke Marsden: I'm thinking it probably makes sense to just have a crack at that this week.
Luke Marsden: Then by next Monday we can take a look at it together.
Luke Marsden: I can also let you loose on the tools that you can use to change it yourselves so that you don't need to ask me for every change.
Luke Marsden: But, but then if I have time as well this week, I'll start thinking about probably the internal agents because I think we've got the most clarity on those in the conversation.
Luke Marsden: And it sounds like Tony's mechanical suit is the first target there.
Tony: Well, as soon as we've got, you know, like a functional website that people can land on and contact us, then like the rose in gym thing doesn't need to be in place initially, but as long as we've got something, go to like a business development message then.
Tony: Yeah, good to go on that.
Tony: We, you know, we spin up the Gmail and link it and all that kind of stuff.
Luke Marsden: Okay, super.
Luke Marsden: Yeah.
Luke Marsden: And you've got the domain, have you for it?
Luke Marsden: Yes.
Luke Marsden: Pre reg, you said are you okay?
Tony: Sorry, we find AI, I think it is.
Tony: Or we hyphen find or something like that.
Luke Marsden: Okay, cool, that's fine.
Luke Marsden: And then are you okay to set up the Gmail yourself or do you need help with that?
Tony: Yeah, I can do it.
Tony: I know it's a bit fiddly, isn't it?
Luke Marsden: With all this MX Records and stuff.
Luke Marsden: Copy paste.
Tony: Yeah, yeah, I can do that.
Tony: Leah keeps asking me to.
Luke Marsden: To do that.
Tony: I'll keep putting it off because I know it's really annoying, but I'll do that.
Luke Marsden: Yeah, that's cool.
Luke Marsden: And I'll set up the hosting on our side.
Luke Marsden: That should be fairly straightforward.
Luke Marsden: And.
Leah Smith: Did you receive.
Leah Smith: Sorry, Luke, I did send you the brand guidelines.
Luke Marsden: Yeah, I saw those.
Luke Marsden: Fine, yeah, yeah.
Leah Smith: Do you need anything else from our side?
Luke Marsden: No, I got.
Luke Marsden: I got the brief, I think from the last conversation, which was basically like a cut down version of the Linux Recruit site with the new branding and the new name.
Luke Marsden: If you want to send through any notes on messaging, then you can do that now just on email or I'll just put something in place.
Luke Marsden: It will just be like.
Luke Marsden: I mean, AI is going to build it for me so the words don't really matter that much and then I give you access to start changing them and tweaking.
Luke Marsden: But yeah, that should be fine.
Tony: And what's the best way to kind of communicate?
Tony: Would you want to set up like a Slack channel?
Luke Marsden: I was going to say let's set up a Slack connect.
Luke Marsden: I see you're already using Slack, so our invite will come from the MLOps community.
Luke Marsden: Slack.
Luke Marsden: Okay, because that's the one where we hang out, so don't be surprised when it comes up at that name.
Luke Marsden: And we can make a private channel on there.
Luke Marsden: So.
Luke Marsden: Yeah, yeah, brilliant.
Luke Marsden: Okay, great.
Tony: Awesome.
Luke Marsden: Well, we'll get cracking then.
Luke Marsden: Chris, would you mind sending a Slack invite?
Luke Marsden: Would that be okay?
Leah Smith: You?
Chris Sterry: Yeah, no problem.
Luke Marsden: Please.
Luke Marsden: Thank you.
Luke Marsden: Awesome.
Luke Marsden: Brilliant.
Luke Marsden: I'll just click make this meeting recurring then and I'll see you next Monday and we can chat on Slack in the meantime.
Tony: Perfect.
Luke Marsden: All right, cheers guys.
Luke Marsden: Thanks, Chris.

