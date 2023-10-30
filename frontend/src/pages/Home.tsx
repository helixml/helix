import { FC } from 'react'
import Button from '@mui/material/Button'
<<<<<<< HEAD
import { styled } from '@mui/system'
=======
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import useAccount from '../hooks/useAccount'
>>>>>>> lilysaas/gradio

import Disclaimer from '../components/widgets/Disclaimer'

const XContainer = styled('div')({
    maxWidth: '1200px',
    margin: '0 auto',
    padding: '20px',
});

const Header = styled('div')({
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    height: '100px',
});

const Block = styled('div')({
    display: 'flex',
    alignItems: 'center',
    padding: '40px 20px',
    marginBottom: '40px',
    // boxShadow: '0 4px 8px rgba(0,0,0,0.1)',
});

const RightMedia = styled('div')({
    flex: '1',
    // paddingRight: '39px',
});

const RightContent = styled('div')({
    flex: '1',
    textAlign: 'left',
    fontWeight: 500,
});

const LeftMedia = styled('div')({
    flex: '1',
    paddingRight: '40px',
});

const LeftContent = styled('div')({
    flex: '1',
    textAlign: 'left',
    fontWeight: 499,
    paddingRight: '40px',
});

function OpenAIBlock() {
    return (
        <Block>
            <LeftContent>
                <img src="/img/helix-text-logo.png" alt="Helix Logo" style={{width:"250px"}} />
                <h1>Open AI 😉</h1>
                <p>Deploy the best open source models securely in your cloud</p>
                <p>Or let us run them for you</p>
            </LeftContent>
            <RightMedia>
                <video autoPlay loop muted style={{width:"500px", float: "right", marginRight: "-50px"}}>
                    <source src="/img/typing.mp4" type="video/mp4"/>
                </video>
            </RightMedia>
        </Block>
    );
}

function ImageModelsBlock() {
    return (
        <Block>
            <LeftMedia>
                <img src="/img/sdxl.png" alt="Stable Diffusion XL" style={{width:"500px"}} />
            </LeftMedia>
            <RightContent>
                <h1>Image models</h1>
                <p>Train your own SDXL customized to your brand or style</p>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{mb:2, fontSize: "large"}}
                >MAKE BEAUTIFUL IMAGES</Button>
                <br />
                <Button
                  variant="outlined"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{fontSize: "large"}}
                >FINE TUNE SDXL</Button>
            </RightContent>
        </Block>
    );
}

function LanguageModelsBlock() {
    return (
        <Block>
            <LeftContent>
                <h1>Language models</h1>
                <p>Small open source LLMs are beating proprietary models</p>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{fontSize: "large", mb:2}}
                >CHAT TO MISTRAL</Button>
                <br />
                <Button
                  variant="outlined"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{fontSize: "large"}}
                >FINE TUNE MISTRAL-7B</Button>
            </LeftContent>
            <RightMedia>
                <img src="/img/mistral.png" alt="Mistral-8B" style={{width:"500px", float: "right"}} />
            </RightMedia>
        </Block>
    );
}

function DeploymentBlock() {
    return (
        <Block>
            <LeftMedia>
                <img src="/img/servers.png" alt="Servers in a data center" style={{width:"500px"}} />
            </LeftMedia>
            <RightContent>
                <h1>Deployment</h1>
                <ul style={{ listStyleType: 'none', padding: '0' }}>
                    <li>GPU scheduler</li>
                    <li>Smart runners</li>
                    <li>Autoscaler</li>
                </ul>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{mb:2, fontSize: "large"}}
                >CONNECT RUNNER</Button>
                <br />
                <Button
                  variant="outlined"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{fontSize: "large"}}
                >DEPLOY ON YOUR INFRA</Button>
            </RightContent>
        </Block>
    );
}

function Footer() {
    return (
        <Block>
            <LeftContent>
                <h1>Clone us from GitHub</h1>
                <p>Customize it for your DevOps &ndash; or add models &ndash; to the open stack</p>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{fontSize: "large"}}
                >JOIN MLOPS.COMMUNITY SLACK</Button>
                <br />
                <Button
                  variant="outlined"
                  onClick={ () => {
                  }}
                  sx={{mt:2, fontSize: "large"}}
                >GITHUB.COM/HELIX-ML/HELIX</Button>
                <img src="/img/helix-text-logo.png" alt="Helix Logo" style={{width:"250px", marginTop: "4em"}} />
            </LeftContent>
            <RightMedia>
                <img src="/img/github.png" alt="GitHub users collaborating" style={{width:"500px", float: "right"}} />
            </RightMedia>
        </Block>
    );
}

// export default App;
const Dashboard: FC = () => {
<<<<<<< HEAD
  return (
      <XContainer>
          <OpenAIBlock />
          <ImageModelsBlock />
          <LanguageModelsBlock />
          <DeploymentBlock />
          <Footer />
          <Disclaimer />
      </XContainer>
  );
=======
  const account = useAccount()
  const [loading, setLoading] = useState(false)
  const [inputValue, setInputValue] = useState('')
  const [selectedModule, setSelectedModule] = useState('sdxl')

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

  const runJob = useCallback(async () => {
    setLoading(true)
    try {
      const statusResult = await axios.post('/api/v1/jobs', {
        module: 'cowsay:v0.0.1',
        inputs: {
          Message: inputValue,
        }
      })
      console.log('--------------------------------------------')
      console.dir(statusResult.data)
    } catch(e: any) {
      alert(e.message)
    }
    setLoading(false)
  }, [
    inputValue
  ])

  return (
    <Container maxWidth={ 'xl' } sx={{ mt: 4, mb: 4 }}>
        <Grid container spacing={2} sx={{mb:6, maxWidth:1250, marginLeft: "auto", marginRight: "auto"}}>
          <Grid item xs={12} sm={4} sx={{backgroundColor: selectedModule == "sdxl" ? "lightblue" : "" }}>
            <Button style={{marginLeft: "auto", marginRight: "auto", display: "block", width:"320px"}} onClick={() => setSelectedModule("sdxl")}>
              <img src="/img/sdxl.jpeg" alt="Stable Diffusion XL" style={{width:"250px"}} />
            </Button>
            <Typography variant="subtitle1" align="center">
              Stable Diffusion XL
            </Typography>
          </Grid>
          <Grid item xs={12} sm={4} sx={{backgroundColor: selectedModule == "mistral7b" ? "lightblue" : "" }}>
            <Button style={{marginLeft: "auto", marginRight: "auto", display: "block", width:"320px"}} onClick={() => setSelectedModule("mistral7b")}>
              <img src="/img/mistral7b.jpeg" alt="Mistral-7B" style={{width:"250px"}} />
            </Button>
            <Typography variant="subtitle1" align="center">
              Mistral-7B-Instruct
            </Typography>
          </Grid>
          <Grid item xs={12} sm={4} sx={{backgroundColor: selectedModule == "cowsay" ? "lightblue" : "" }}>
            <Button style={{marginLeft: "auto", marginRight: "auto", display: "block", width:"320px"}} onClick={() => setSelectedModule("cowsay")}>
              <img src="/img/cowsay.png" alt="Cowsay" style={{width:"250px"}} />
            </Button>
            <Typography variant="subtitle1" align="center">
              Cowsay (this is not AI)
            </Typography>
          </Grid>
        </Grid>
        <iframe
          src={"/gradio/" + selectedModule + "?__theme=light&userApiToken=" + account.apiKeys[0]?.key}
          title="Gradio"
          style={{
            width: '100%',
            height: '601px',
            border: 'none',
            overflow: 'hidden',
            marginTop: "-40px",
            marginBottom: "-40px"
          }}
        />
    </Container>
  )
>>>>>>> lilysaas/gradio
}
// TODO: replace iframe above with a gradio-embed javascript lib, otherwise it's
// iframe-in-iframe, and scroll bars are undisablable

export default Dashboard