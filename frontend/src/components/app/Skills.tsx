import React, { useState, useMemo, useEffect } from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Button, IconButton, Menu, MenuItem, useTheme, Tooltip, Dialog, DialogTitle, DialogContent, DialogContentText, DialogActions, Alert, Collapse, Link, Chip, FormControl, InputLabel, Select, SelectChangeEvent, Tabs, Tab, TextField, InputAdornment } from '@mui/material';
import { PROVIDER_ICONS, PROVIDER_COLORS, SlackLogo } from '../icons/ProviderIcons';
import atlassianLogo from '../../../assets/img/atlassian-logo.png';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import LanguageIcon from '@mui/icons-material/Language';
import CalculateIcon from '@mui/icons-material/Calculate';
import WarningIcon from '@mui/icons-material/Warning';
import CloseIcon from '@mui/icons-material/Close';
import SettingsIcon from '@mui/icons-material/Settings';
import GoogleIcon from '@mui/icons-material/Google';
import MicrosoftIcon from '@mui/icons-material/Microsoft';
import EmailIcon from '@mui/icons-material/Email';
import GitHubIcon from '@mui/icons-material/GitHub';
import LinkedInIcon from '@mui/icons-material/LinkedIn';
import StorageIcon from '@mui/icons-material/Storage';
import SearchIcon from '@mui/icons-material/Search';
import { IAppFlatState, IAgentSkill } from '../../types';
import AddApiSkillDialog from './AddApiSkillDialog';
import BrowserSkill from './BrowserSkill';
import CalculatorSkill from './CalculatorSkill';
import EmailSkill from './EmailSkill';
import ApiIcon from '@mui/icons-material/Api';
import useApi from '../../hooks/useApi';
import useAccount from '../../hooks/useAccount';
import useRouter from '../../hooks/useRouter';

import { alphaVantageTool } from './examples/skillAlphaVantageApi';
import { airQualityTool } from './examples/skillAirQualityApi';
import { exchangeRatesSkill } from './examples/skillExchangeRatesApi';
import WebSearchSkill from './WebSearchSkill';

// OAuth Provider Skills
// import { githubTool } from './examples/skillGithubApi';
// import { gmailTool } from './examples/skillGmailApi';
// import { googleDriveTool } from './examples/skillGoogleDriveApi';
// import { googleCalendarTool } from './examples/skillGoogleCalendarApi';
// import { outlookTool } from './examples/skillOutlookApi';
// import { oneDriveTool } from './examples/skillOneDriveApi';
// import { slackTool } from './examples/skillSlackApi';
// import { linkedInTool } from './examples/skillLinkedInApi';
// import { atlassianTool } from './examples/skillAtlassianApi';
// import { confluenceTool } from './examples/skillConfluenceApi';

// Interface for OAuth provider objects from the API
interface OAuthProvider {
  id: string;
  type: string;
  name: string;
  enabled: boolean;
}

// Interface for OAuth connection objects from the API
interface OAuthConnection {
  id: string;
  providerId: string;
  userId: string;
  expiresAt: string;
  provider?: {
    name: string;
    type: string;
  };
}

interface ISkill {
  id: string;
  icon?: React.ReactNode;
  name: string;
  description: string;
  type: string;
  category?: string;
  skill: IAgentSkill;
}

const SKILL_TYPE_HTTP_API = 'HTTP API';
const SKILL_TYPE_BROWSER = 'Browser';
const SKILL_TYPE_WEB_SEARCH = 'Web Search';
const SKILL_TYPE_CALCULATOR = 'Calculator';
const SKILL_TYPE_EMAIL = 'Email';

const SKILL_CATEGORY_CORE = 'Core';
const SKILL_CATEGORY_DATA = 'Data & APIs';
const SKILL_CATEGORY_GOOGLE = 'Google';
const SKILL_CATEGORY_MICROSOFT = 'Microsoft';
const SKILL_CATEGORY_GITHUB = 'GitHub';
const SKILL_CATEGORY_SLACK = 'Slack';
const SKILL_CATEGORY_LINKEDIN = 'LinkedIn';
const SKILL_CATEGORY_ATLASSIAN = 'Atlassian';

// Function to get category icon
const getCategoryIcon = (category: string) => {
  switch (category) {
    case 'All':
      return <SettingsIcon sx={{ fontSize: 16 }} />;
    case SKILL_CATEGORY_CORE:
      return <SettingsIcon sx={{ fontSize: 16 }} />;
    case SKILL_CATEGORY_DATA:
      return <StorageIcon sx={{ fontSize: 16 }} />;
    case SKILL_CATEGORY_GOOGLE:
      return <GoogleIcon sx={{ fontSize: 16, color: '#4285F4' }} />;
    case SKILL_CATEGORY_MICROSOFT:
      return <MicrosoftIcon sx={{ fontSize: 16, color: '#00A1F1' }} />;
    case SKILL_CATEGORY_GITHUB:
      return <GitHubIcon sx={{ fontSize: 16 }} />;
    case SKILL_CATEGORY_SLACK:
      return <SlackLogo sx={{ fontSize: 16 }} />;
    case SKILL_CATEGORY_LINKEDIN:
      return <LinkedInIcon sx={{ fontSize: 16, color: '#0077B5' }} />;
    case SKILL_CATEGORY_ATLASSIAN:
      return <img src={atlassianLogo} style={{ width: 16, height: 16 }} alt="Atlassian" />;
    default:
      return <SettingsIcon sx={{ fontSize: 16 }} />;
  }
};

// Base static skills/plugins data
const BASE_SKILLS: ISkill[] = [
  {
    id: 'browser',
    icon: <LanguageIcon />,
    name: 'Browser',
    description: 'Enable the AI to browse websites and extract information from them. The AI can visit URLs and process their content.',
    type: SKILL_TYPE_BROWSER,
    category: SKILL_CATEGORY_CORE,
    skill: {
      name: 'Browser',
      description: 'Enable the AI to browse websites and extract information from them.',
      systemPrompt: '',
      apiSkill: {
        schema: '',
        url: '',
        requiredParameters: [],
      },
      configurable: true,
    },
  },
  {
    id: 'web-search',
    icon: <SearchIcon />,
    name: 'Web Search',
    description: 'Enable the AI to search the web for current information. Can be used to build deep search style agents.',
    type: SKILL_TYPE_WEB_SEARCH,
    category: SKILL_CATEGORY_CORE,
    skill: {
      name: 'Web Search',
      description: 'Enable the AI to search the web for current information.',
      systemPrompt: '',
      apiSkill: {
        schema: '',
        url: '',
        requiredParameters: [],
      },
      configurable: true,
    },
  },
  {
    id: 'calculator',
    icon: <CalculateIcon />,
    name: 'Calculator',
    description: 'Enable the AI to perform math calculations using javascript expressions.',
    type: SKILL_TYPE_CALCULATOR,
    category: SKILL_CATEGORY_CORE,
    skill: {
      name: 'Calculator',
      description: 'Enable the AI to perform math calculations.',
      systemPrompt: '',
      apiSkill: {
        schema: '',
        url: '',
        requiredParameters: [],
      },
      configurable: true,
    },
  },
  {
    id: 'email',
    icon: <EmailIcon />,
    name: 'Email',
    description: 'Allow agent to send emails to you. Instruct it to send summaries, reminders, or other information via email.',
    type: SKILL_TYPE_EMAIL,
    skill: {
      name: 'Email',
      description: 'Enable the AI to send emails to the user.',
      systemPrompt: '',
      apiSkill: {
        schema: '',
        url: '',
        requiredParameters: [],
      },
      configurable: true,
    },
  },
  {
    id: 'alpha-vantage',
    icon: alphaVantageTool.icon,
    name: alphaVantageTool.name,
    description: alphaVantageTool.description,
    type: SKILL_TYPE_HTTP_API,
    category: SKILL_CATEGORY_DATA,
    skill: alphaVantageTool,
  },
  {
    id: 'air-quality',
    icon: airQualityTool.icon,
    name: airQualityTool.name,
    description: airQualityTool.description,
    type: SKILL_TYPE_HTTP_API,
    category: SKILL_CATEGORY_DATA,
    skill: airQualityTool,
  },
  {
    id: 'exchange-rates',
    icon: exchangeRatesSkill.icon,
    name: exchangeRatesSkill.name,
    description: exchangeRatesSkill.description,
    type: SKILL_TYPE_HTTP_API,
    category: SKILL_CATEGORY_DATA,
    skill: exchangeRatesSkill,
  },
  // OAuth Provider Skills
  // {
  //   id: 'github-api',
  //   icon: githubTool.icon,
  //   name: githubTool.name,
  //   description: githubTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_GITHUB,
  //   skill: githubTool,
  // },
  // {
  //   id: 'gmail-api',
  //   icon: gmailTool.icon,
  //   name: gmailTool.name,
  //   description: gmailTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_GOOGLE,
  //   skill: gmailTool,
  // },
  // {
  //   id: 'google-drive-api',
  //   icon: googleDriveTool.icon,
  //   name: googleDriveTool.name,
  //   description: googleDriveTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_GOOGLE,
  //   skill: googleDriveTool,
  // },
  // {
  //   id: 'google-calendar-api',
  //   icon: googleCalendarTool.icon,
  //   name: googleCalendarTool.name,
  //   description: googleCalendarTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_GOOGLE,
  //   skill: googleCalendarTool,
  // },
  // {
  //   id: 'outlook-api',
  //   icon: outlookTool.icon,
  //   name: outlookTool.name,
  //   description: outlookTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_MICROSOFT,
  //   skill: outlookTool,
  // },
  // {
  //   id: 'onedrive-api',
  //   icon: oneDriveTool.icon,
  //   name: oneDriveTool.name,
  //   description: oneDriveTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_MICROSOFT,
  //   skill: oneDriveTool,
  // },
  // {
  //   id: 'slack-api',
  //   icon: slackTool.icon,
  //   name: slackTool.name,
  //   description: slackTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_SLACK,
  //   skill: slackTool,
  // },
  // {
  //   id: 'linkedin-api',
  //   icon: linkedInTool.icon,
  //   name: linkedInTool.name,
  //   description: linkedInTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_LINKEDIN,
  //   skill: linkedInTool,
  // },
  // {
  //   id: 'atlassian-api',
  //   icon: atlassianTool.icon,
  //   name: atlassianTool.name,
  //   description: atlassianTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_ATLASSIAN,
  //   skill: atlassianTool,
  // },
  // {
  //   id: 'confluence-api',
  //   icon: confluenceTool.icon,
  //   name: confluenceTool.name,
  //   description: confluenceTool.description,
  //   type: SKILL_TYPE_HTTP_API,
  //   category: SKILL_CATEGORY_ATLASSIAN,
  //   skill: confluenceTool,
  // },
];

const CUSTOM_API_SKILL: ISkill = {
  id: 'new-custom-api',
  icon: <ApiIcon />,
  name: 'New API',
  description: 'Add your own OpenAPI based integration. Any HTTP endpoint can become a skill for your agent.',
  type: SKILL_TYPE_HTTP_API,
  category: SKILL_CATEGORY_CORE,
  skill: {
    name: 'Custom API',
    icon: <ApiIcon />,
    description: 'Add your own API integration.',
    systemPrompt: '',
    apiSkill: {
      schema: '',
      url: '',
      requiredParameters: [],
    },
    configurable: true,
  },
};

const getFirstLine = (text: string): string => {
  return text.split('\n')[0].trim();
};

interface SkillsProps {
  app: IAppFlatState,
  onUpdate: (updates: IAppFlatState) => Promise<void>,
}

const Skills: React.FC<SkillsProps> = ({ 
  app, 
  onUpdate,
}) => {
  const theme = useTheme();
  const api = useApi();
  const account = useAccount();
  const router = useRouter();

  const [selectedSkill, setSelectedSkill] = useState<IAgentSkill | null>(null);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [selectedSkillForMenu, setSelectedSkillForMenu] = useState<string | null>(null);
  const [isDisableConfirmOpen, setIsDisableConfirmOpen] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState<string>('All');
  const [skillToDisable, setSkillToDisable] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState<string>('');
  
  // OAuth warning state
  const [oauthProviders, setOAuthProviders] = useState<OAuthProvider[]>([]);
  const [oauthConnections, setOAuthConnections] = useState<OAuthConnection[]>([]);
  const [missingProviders, setMissingProviders] = useState<string[]>([]);
  const [showWarning, setShowWarning] = useState(false);

  // Fetch OAuth providers and connections
  useEffect(() => {
    const fetchOAuthData = async () => {
      try {
        const [providersResponse, connectionsResponse] = await Promise.all([
          api.get('/api/v1/oauth/providers'),
          api.get('/api/v1/oauth/connections')
        ]);
        
        const providers = Array.isArray(providersResponse) ? providersResponse : [];
        const connections = Array.isArray(connectionsResponse) ? connectionsResponse : [];
        
        setOAuthProviders(providers);
        setOAuthConnections(connections);
      } catch (error) {
        console.error('Error fetching OAuth data:', error);
        setOAuthProviders([]);
        setOAuthConnections([]);
      }
    };

    fetchOAuthData();
  }, []);

  // Check for missing OAuth providers whenever app.apiTools changes
  useEffect(() => {
    if (!app.apiTools || app.apiTools.length === 0) {
      setMissingProviders([]);
      setShowWarning(false);
      return;
    }

    const requiredProviders = new Set<string>();
    
    // Collect all OAuth providers required by API tools
    app.apiTools.forEach(tool => {
      if (tool.oauth_provider && tool.oauth_provider.trim() !== '') {
        requiredProviders.add(tool.oauth_provider);
      }
    });

    if (requiredProviders.size === 0) {
      setMissingProviders([]);
      setShowWarning(false);
      return;
    }

    const enabledProviderNames = new Set(
      oauthProviders.filter(p => p.enabled).map(p => p.name)
    );
    
    const connectedProviderNames = new Set(
      oauthConnections.map(c => {
        const provider = oauthProviders.find(p => p.id === c.providerId);
        return provider?.name || '';
      }).filter(name => name !== '')
    );

    const missing: string[] = [];
    
    requiredProviders.forEach(providerName => {
      const isProviderEnabled = enabledProviderNames.has(providerName);
      const isUserConnected = connectedProviderNames.has(providerName);
      
      if (!isProviderEnabled || !isUserConnected) {
        missing.push(providerName);
      }
    });

    setMissingProviders(missing);
    setShowWarning(missing.length > 0);
  }, [app.apiTools, oauthProviders, oauthConnections]);

  // Convert custom APIs to skills
  const customApiSkills = useMemo(() => {
    if (!app.apiTools) return [];

    // Filter out any API tools that match predefined skills
    return app.apiTools
      .filter(api => !BASE_SKILLS.some(skill => skill.name === api.name))
      .map(api => {
        // Determine category based on OAuth provider
        let category = SKILL_CATEGORY_DATA;
        if (api.oauth_provider) {
          switch (api.oauth_provider.toLowerCase()) {
            case 'google':
              category = SKILL_CATEGORY_GOOGLE;
              break;
            case 'microsoft':
              category = SKILL_CATEGORY_MICROSOFT;
              break;
            case 'github':
              category = SKILL_CATEGORY_GITHUB;
              break;
            case 'slack':
              category = SKILL_CATEGORY_SLACK;
              break;
            case 'linkedin':
              category = SKILL_CATEGORY_LINKEDIN;
              break;
            case 'atlassian':
              category = SKILL_CATEGORY_ATLASSIAN;
              break;
            default:
              category = SKILL_CATEGORY_DATA;
          }
        }

        return {
          id: `custom-api-${api.name}`,
          icon: <ApiIcon />,
          name: api.name,
          description: api.description,
          type: SKILL_TYPE_HTTP_API,
          category,
          skill: {
            name: api.name,
            icon: <ApiIcon />,
            description: api.description,
            systemPrompt: api.system_prompt || '',
            apiSkill: {
              schema: api.schema,
              url: api.url,
              requiredParameters: [],
              headers: api.headers || {},
              query: api.query || {},
              oauth_provider: api.oauth_provider || '',
              oauth_scopes: api.oauth_scopes || [],
            },
            configurable: true,
          },
        };
      });
  }, [app.apiTools]);

  // All skills are now shown to everyone
  const allSkills = useMemo(() => {
    return [...BASE_SKILLS, ...customApiSkills, CUSTOM_API_SKILL];
  }, [customApiSkills]);

  const availableCategories = useMemo(() => {
    const categories = new Set<string>();
    allSkills.forEach(skill => {
      if ('category' in skill && skill.category) {
        categories.add(skill.category);
      }
    });
    return ['All', ...Array.from(categories).sort()];
  }, [allSkills]);

  const filteredSkills = useMemo(() => {
    let skills = allSkills;
    
    // Apply search filter first
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase().trim();
      skills = skills.filter(skill => 
        skill.name.toLowerCase().includes(query) ||
        skill.description.toLowerCase().includes(query) ||
        (skill.skill.apiSkill?.oauth_provider && skill.skill.apiSkill.oauth_provider.toLowerCase().includes(query))
      );
    }
    
    // Then apply category filter (but not when searching)
    if (!searchQuery.trim() && selectedCategory !== 'All') {
      return skills.filter(skill => 'category' in skill && skill.category === selectedCategory);
    }
    
    return skills;
  }, [allSkills, selectedCategory, searchQuery]);

  // Auto-switch to category with search results
  useEffect(() => {
    if (searchQuery.trim() && filteredSkills.length > 0) {
      // Find which categories have matching skills
      const categoriesWithResults = new Set(
        filteredSkills
          .filter(skill => skill.category)
          .map(skill => skill.category!)
      );
      
      // If all results are in one category, switch to it
      if (categoriesWithResults.size === 1) {
        const singleCategory = Array.from(categoriesWithResults)[0];
        if (singleCategory !== selectedCategory) {
          setSelectedCategory(singleCategory);
        }
      } else if (categoriesWithResults.size > 1) {
        // Multiple categories have results, switch to "All" to show everything
        if (selectedCategory !== 'All') {
          setSelectedCategory('All');
        }
      }
    }
  }, [searchQuery, filteredSkills, selectedCategory]);

  // Helper function to check if a category has enabled skills for this agent
  const getCategorySkillStatus = (category: string) => {
    const categorySkills = allSkills.filter(skill => skill.category === category);
    const enabledSkills = categorySkills.filter(skill => isSkillEnabled(skill.name));
    
    if (enabledSkills.length === categorySkills.length) {
      // All skills in category are enabled - green
      return 'all-enabled';
    } else if (enabledSkills.length > 0) {
      // Some skills in category are enabled - orange
      return 'some-enabled';
    } else {
      // No skills in category are enabled - default blue
      return 'none-enabled';
    }
  };

  // Helper function to get badge colors based on skill enablement status
  const getBadgeColors = (status: string) => {
    switch (status) {
      case 'all-enabled':
        return { bgcolor: '#4caf50', color: '#fff' }; // Green - all skills enabled
      case 'some-enabled':
        return { bgcolor: '#ff9800', color: '#fff' }; // Orange - some skills enabled
      case 'none-enabled':
      default:
        return { bgcolor: 'primary.main', color: 'primary.contrastText' }; // Default blue
    }
  };

  // State for OAuth provider dialog
  const [showOAuthProviderDialog, setShowOAuthProviderDialog] = useState(false);
  const [selectedOAuthProvider, setSelectedOAuthProvider] = useState<string>('');

  const isSkillEnabled = (skillName: string): boolean => {    
    if (skillName === 'Web Search') {
      console.log(app.webSearchTool)
      return app.webSearchTool?.enabled ?? false;
    }
    if (skillName === 'Browser') {
      return app.browserTool?.enabled ?? false;
    }
    if (skillName === 'Calculator') {
      return app.calculatorTool?.enabled ?? false;
    }
    if (skillName === 'Email') {
      return app.emailTool?.enabled ?? false;
    }
    return app.apiTools?.some(tool => tool.name === skillName) ?? false;
  };

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, skillName: string) => {
    setMenuAnchorEl(event.currentTarget);
    setSelectedSkillForMenu(skillName);
  };

  const handleMenuClose = () => {
    setMenuAnchorEl(null);
    setSelectedSkillForMenu(null);
  };

  const handleDisableSkill = async () => {
    if (skillToDisable) {
      const skill = allSkills.find(s => s.name === skillToDisable);
      if (skill) {
        // If skill is Browser, we need to disable the browser tool
        if (skill.name === 'Browser') {
          await onUpdate({
            ...app,
            browserTool: { enabled: false, markdown_post_processing: false },
          });
          return
        }
        if (skill.name === 'Web Search') {
          await onUpdate({
            ...app,
            webSearchTool: { enabled: false, max_results: 10 },
          });
          return
        }
        if (skill.name === 'Calculator') {
          await onUpdate({
            ...app,
            calculatorTool: { enabled: false },
          });
          return
        }
        if (skill.name === 'Email') {
          await onUpdate({
            ...app,
            emailTool: { enabled: false },
          });
          return
        }
        // Remove the tool from app.apiTools
        const updatedTools = app.apiTools?.filter(tool => tool.name !== skill.name) || [];
        
        // Update the app state
        await onUpdate({
          ...app,
          apiTools: updatedTools
        });
      }
    }
    handleMenuClose();
    setIsDisableConfirmOpen(false);
    setSkillToDisable(null);
  };

  const handleDisableClick = () => {
    setSkillToDisable(selectedSkillForMenu);
    setIsDisableConfirmOpen(true);
    handleMenuClose();
  };

  const handleOpenDialog = (skill: ISkill) => {
    // Check if this is an OAuth skill with disabled provider for regular users
    const oauthProvider = skill.skill.apiSkill?.oauth_provider;
    if (oauthProvider && !account.admin) {
      const provider = oauthProviders.find(p => p.name === oauthProvider);
      if (!provider || !provider.enabled) {
        // Show OAuth provider dialog for regular users
        setSelectedOAuthProvider(oauthProvider);
        setShowOAuthProviderDialog(true);
        return;
      }
    }

    // For custom API tile, don't pass the skill template
    if (skill.id === 'new-custom-api') {
      setSelectedSkill(null);
    } else {
      setSelectedSkill(skill.skill);
    }
    setIsDialogOpen(true);
  };

  const renderSkillDialog = () => {
    if (!selectedSkill) {
      return (
        <AddApiSkillDialog
          open={isDialogOpen}
          onClose={() => {
            setIsDialogOpen(false);
          }}
          onClosed={() => {
            setSelectedSkill(null);
          }}
          skill={undefined}
          app={app}
          onUpdate={onUpdate}
          isEnabled={false}
        />
      );
    }

    if (selectedSkill.name === 'Browser') {
      return (
        <BrowserSkill
          open={isDialogOpen}
          onClose={() => {
            setIsDialogOpen(false);
          }}
          onClosed={() => {
            setSelectedSkill(null);
          }}
          app={app}
          onUpdate={onUpdate}
          isEnabled={isSkillEnabled('Browser')}
        />
      );
    }

    if (selectedSkill.name === 'Web Search') {
      return (
        <WebSearchSkill
          open={isDialogOpen}
          onClose={() => {
            setIsDialogOpen(false);
          }}
          onClosed={() => {
            setSelectedSkill(null);
          }}
          app={app}
          onUpdate={onUpdate}
          isEnabled={isSkillEnabled('Web Search')}
        />
      );
    } 

    if (selectedSkill.name === 'Calculator') {
      return (
        <CalculatorSkill
          open={isDialogOpen}
          onClose={() => {
            setIsDialogOpen(false);
          }}
          onClosed={() => {
            setSelectedSkill(null);
          }}
          app={app}
          onUpdate={onUpdate}
          isEnabled={isSkillEnabled('Calculator')}
        />
      );
    }

    if (selectedSkill.name === 'Email') {
      return (
        <EmailSkill
          open={isDialogOpen}
          onClose={() => {
            setIsDialogOpen(false);
          }}
          onClosed={() => {
            setSelectedSkill(null);
          }}
          app={app}
          onUpdate={onUpdate}
          isEnabled={isSkillEnabled('Email')}
        />
      );
    }

    return (
      <AddApiSkillDialog
        open={isDialogOpen}
        onClose={() => {
          setIsDialogOpen(false);
        }}
        onClosed={() => {
          setSelectedSkill(null);
        }}
        skill={selectedSkill}
        app={app}
        onUpdate={onUpdate}
        isEnabled={isSkillEnabled(selectedSkill.name)}
      />
    );
  };

  return (
    <Box sx={{ mt: 2, mr: 4 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        💡 Skills
      </Typography>
      {/* Add a paragraph with info about skills */}
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Extend the capabilities of the AI with custom functions, APIs and workflows.
      </Typography>

      {/* Search and Category Tabs */}
      <Box sx={{ mb: 3 }}>
        {/* Search Bar */}
        <Box sx={{ mb: 2, display: 'flex', alignItems: 'center', gap: 2 }}>
          <TextField
            size="small"
            placeholder="Search skills..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon sx={{ fontSize: 20 }} />
                </InputAdornment>
              ),
            }}
            sx={{ 
              flexGrow: 1,
              maxWidth: 400,
              '& .MuiOutlinedInput-root': {
                bgcolor: 'background.paper',
              },
            }}
          />
          {searchQuery && (
            <Typography variant="body2" color="text.secondary">
              {filteredSkills.length === 0 ? 'No results found' : `${filteredSkills.length} result${filteredSkills.length === 1 ? '' : 's'}`}
            </Typography>
          )}
        </Box>

        {/* No Results Suggestion */}
        {searchQuery && filteredSkills.length === 0 && (
          <Alert 
            severity="info" 
            sx={{ mb: 2 }}
            action={
              <Button 
                size="small" 
                onClick={() => handleOpenDialog(CUSTOM_API_SKILL)}
                sx={{ textTransform: 'none' }}
              >
                Add Custom API
              </Button>
            }
          >
            <Typography variant="body2">
              Can't find what you're looking for? Try adding a custom API integration for "{searchQuery}".
            </Typography>
          </Alert>
        )}

        {/* Category Tabs */}
        {!searchQuery.trim() && (
          <Tabs
            value={selectedCategory}
            onChange={(event, newValue) => setSelectedCategory(newValue)}
            variant="scrollable"
            scrollButtons="auto"
            sx={{
              '& .MuiTabs-flexContainer': {
                gap: 0.75,
              },
              '& .MuiTab-root': {
                textTransform: 'none',
                minHeight: 44,
                minWidth: 60,
                fontWeight: 500,
                fontSize: '0.85rem',
                px: 1.5,
                '&.Mui-selected': {
                  fontWeight: 600,
                },
              },
            }}
          >
            {/* All tab first */}
            <Tab
              key="All"
              value="All"
              label={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                  <SettingsIcon sx={{ fontSize: 16 }} />
                  <Typography variant="body2" sx={{ fontWeight: 'inherit' }}>
                    All
                  </Typography>
                  <Chip 
                    label={allSkills.length} 
                    size="small" 
                    sx={{ 
                      minWidth: '18px',
                      height: '18px',
                      fontSize: '0.65rem',
                      bgcolor: 'primary.main',
                      color: 'primary.contrastText',
                      '& .MuiChip-label': { px: 0.6 }
                    }} 
                  />
                </Box>
              }
            />

            {/* Core tab second */}
            <Tab
              key={SKILL_CATEGORY_CORE}
              value={SKILL_CATEGORY_CORE}
              label={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                  {getCategoryIcon(SKILL_CATEGORY_CORE)}
                  <Typography variant="body2" sx={{ fontWeight: 'inherit' }}>
                    Core
                  </Typography>
                  <Chip 
                    label={allSkills.filter(skill => skill.category === SKILL_CATEGORY_CORE).length} 
                    size="small" 
                    sx={{ 
                      minWidth: '18px',
                      height: '18px',
                      fontSize: '0.65rem',
                      ...getBadgeColors(getCategorySkillStatus(SKILL_CATEGORY_CORE)),
                      '& .MuiChip-label': { px: 0.6 }
                    }} 
                  />
                </Box>
              }
            />
            
            {/* Data & APIs third */}
            <Tab
              key={SKILL_CATEGORY_DATA}
              value={SKILL_CATEGORY_DATA}
              label={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                  {getCategoryIcon(SKILL_CATEGORY_DATA)}
                  <Typography variant="body2" sx={{ fontWeight: 'inherit' }}>
                    Data & APIs
                  </Typography>
                  <Chip 
                    label={allSkills.filter(skill => skill.category === SKILL_CATEGORY_DATA).length} 
                    size="small" 
                    sx={{ 
                      minWidth: '18px',
                      height: '18px',
                      fontSize: '0.65rem',
                      ...getBadgeColors(getCategorySkillStatus(SKILL_CATEGORY_DATA)),
                      '& .MuiChip-label': { px: 0.6 }
                    }} 
                  />
                </Box>
              }
            />
            
            {/* Provider-specific categories */}
            {availableCategories
              .filter(cat => cat !== 'All' && cat !== SKILL_CATEGORY_CORE && cat !== SKILL_CATEGORY_DATA)
              .map(category => {
                const skillCount = allSkills.filter(skill => skill.category === category).length;
                // Shorten category names for better fit
                const shortName = category === SKILL_CATEGORY_ATLASSIAN ? 'Atlassian' : category;
                return (
                  <Tab
                    key={category}
                    value={category}
                    label={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                        {getCategoryIcon(category)}
                        <Typography variant="body2" sx={{ fontWeight: 'inherit' }}>
                          {shortName}
                        </Typography>
                        <Chip 
                          label={skillCount} 
                          size="small" 
                          sx={{ 
                            minWidth: '18px',
                            height: '18px',
                            fontSize: '0.65rem',
                            ...getBadgeColors(getCategorySkillStatus(category)),
                            '& .MuiChip-label': { px: 0.6 }
                          }} 
                        />
                      </Box>
                    }
                  />
                );
              })}
          </Tabs>
        )}
      </Box>

      {/* OAuth Provider Warning Banner */}
      <Collapse in={showWarning}>
        <Alert
          severity="warning"
          icon={<WarningIcon />}
          action={
            <IconButton
              aria-label="close"
              color="inherit"
              size="small"
              onClick={() => setShowWarning(false)}
            >
              <CloseIcon fontSize="inherit" />
            </IconButton>
          }
          sx={{ mb: 2 }}
        >
          <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 1 }}>
            OAuth Configuration Required
          </Typography>
          <Typography variant="body2" sx={{ mb: 1 }}>
            This agent requires OAuth providers that are not properly configured:
          </Typography>
          <Box sx={{ mb: 1 }}>
            {missingProviders.map((providerName, index) => {
              const provider = oauthProviders.find(p => p.name === providerName);
              const isProviderEnabled = provider?.enabled || false;
              const isUserConnected = oauthConnections.some(c => {
                const connectedProvider = oauthProviders.find(p => p.id === c.providerId);
                return connectedProvider?.name === providerName;
              });

              return (
                <Box key={providerName} sx={{ display: 'flex', alignItems: 'center', mb: 0.5 }}>
                  <Typography variant="body2" sx={{ mr: 1 }}>
                    • <strong>{providerName}</strong>:
                  </Typography>
                  {!isProviderEnabled ? (
                    <Typography variant="body2" color="error.main">
                      Not configured by administrator
                    </Typography>
                  ) : !isUserConnected ? (
                    <Link
                      href="/oauth-connections"
                      sx={{ textDecoration: 'underline', cursor: 'pointer' }}
                      onClick={(e) => {
                        e.preventDefault();
                        // Navigate to account page OAuth section
                        window.location.href = '/oauth-connections';
                      }}
                    >
                      Connect your account
                    </Link>
                  ) : null}
                </Box>
              );
            })}
          </Box>
          <Typography variant="body2">
            {missingProviders.some(name => {
              const provider = oauthProviders.find(p => p.name === name);
              return !provider?.enabled;
            }) && (
              <>
                Administrators can configure OAuth providers in{' '}
                <Link
                  sx={{ textDecoration: 'underline', cursor: 'pointer' }}
                  onClick={(e) => {
                    e.preventDefault();
                    router.navigate('dashboard', { tab: 'oauth_providers' });
                  }}
                >
                  Dashboard
                </Link>
                .{' '}
              </>
            )}
            You can test this agent once all required OAuth providers are properly configured and connected.
          </Typography>
        </Alert>
      </Collapse>

      <Grid container spacing={2}>
        {filteredSkills.map((skill) => {
          const defaultIcon = PROVIDER_ICONS[skill.type] || PROVIDER_ICONS['custom'];
          const color = PROVIDER_COLORS[skill.type] || PROVIDER_COLORS['custom'];
          const isEnabled = isSkillEnabled(skill.name);
          const isCustomApiTile = skill.id === 'new-custom-api';
          
          // Check OAuth provider status for this skill (admin-only warnings)
          const oauthProvider = skill.skill.apiSkill?.oauth_provider;
          const provider = oauthProvider ? oauthProviders.find(p => p.name === oauthProvider) : null;
          const isOAuthSkill = !!oauthProvider;
          const isProviderDisabled = isOAuthSkill && (!provider || !provider.enabled);
          const showProviderWarning = isOAuthSkill && isProviderDisabled && account.admin;
          
          return (
            <Grid item xs={12} sm={6} md={4} lg={3} key={skill.id}>
              <Tooltip
                title={
                  <Box sx={{ p: 1 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 1 }}>
                      Skill Type: {skill.type.toUpperCase()}
                    </Typography>
                    <Typography variant="body2">
                      {skill.description}
                    </Typography>
                  </Box>
                }
                arrow
                placement="bottom"
                componentsProps={{
                  tooltip: {
                    sx: {
                      bgcolor: 'background.paper',
                      color: 'text.primary',
                      border: '1px solid',
                      borderColor: 'divider',
                      boxShadow: 3,
                      maxWidth: 300,
                      '& .MuiTooltip-arrow': {
                        color: 'background.paper',
                        '&:before': {
                          border: '1px solid',
                          borderColor: 'divider',
                        },
                      },
                    },
                  },
                }}
              >
                <Card
                  sx={{
                    height: '100%',
                    display: 'flex',
                    flexDirection: 'column',
                    transition: 'all 0.2s',
                    boxShadow: 2,
                    opacity: isEnabled ? 1 : 0.7,
                    borderStyle: 'dashed',
                    borderWidth: 1,
                    borderColor: 'divider',
                    '&:hover': {
                      transform: isEnabled ? 'translateY(-4px)' : 'none',
                      boxShadow: isEnabled ? 4 : 2,
                    },
                    ...(isCustomApiTile && {
                      position: 'relative',
                      '&::before': {
                        content: '""',
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        right: 0,
                        bottom: 0,
                        borderRadius: 'inherit',
                        padding: '2px',
                        background: 'linear-gradient(45deg, #ff6b6b, #4ecdc4, #45b7d1, #96c93d)',
                        WebkitMask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
                        WebkitMaskComposite: 'xor',
                        maskComposite: 'exclude',
                        animation: 'shimmer 3s linear infinite',
                        backgroundSize: '300% 300%',
                      },
                      '@keyframes shimmer': {
                        '0%': {
                          backgroundPosition: '0% 50%',
                        },
                        '50%': {
                          backgroundPosition: '100% 50%',
                        },
                        '100%': {
                          backgroundPosition: '0% 50%',
                        },
                      },
                    }),
                  }}
                >
                  <CardHeader
                    avatar={
                      <Avatar sx={{ bgcolor: 'white', color: color, width: 40, height: 40 }}>
                        {skill.icon || defaultIcon}
                      </Avatar>
                    }
                    title={skill.name}
                    titleTypographyProps={{ variant: 'h6' }}
                    action={
                      isEnabled && (
                        <IconButton
                          onClick={(e) => handleMenuOpen(e, skill.name)}
                          size="small"
                        >
                          <MoreVertIcon />
                        </IconButton>
                      )
                    }
                  />
                  <CardContent sx={{ flexGrow: 1 }}>
                    <Typography variant="body2" color="text.secondary">
                      {getFirstLine(skill.description)}
                    </Typography>
                    
                    {/* OAuth Provider Warning for Admins */}
                    {showProviderWarning && (
                      <Box sx={{ mt: 2 }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                          <Chip 
                            label="OAuth Provider Disabled" 
                            color="warning" 
                            size="small"
                            sx={{ mr: 1 }}
                          />
                        </Box>
                        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                          Provider "{oauthProvider}" needs to be configured
                        </Typography>
                        <Button
                          size="small"
                          startIcon={<SettingsIcon />}
                          onClick={(e) => {
                            e.stopPropagation();
                            router.navigate('dashboard', { tab: 'oauth_providers' });
                          }}
                          sx={{ 
                            fontSize: '0.75rem',
                            color: '#6366F1',
                            minHeight: 'auto',
                            py: 0.5,
                            px: 1,
                          }}
                        >
                          Configure Provider
                        </Button>
                      </Box>
                    )}
                  </CardContent>
                  <CardActions sx={{ justifyContent: 'center', px: 2, pb: 2 }}>
                    {isEnabled ? (
                      <Button
                        startIcon={<CheckCircleIcon sx={{ color: '#4caf50' }} />}
                        sx={{ 
                          color: theme.palette.success.main,
                          borderColor: theme.palette.success.main,
                          '&:hover': {
                            borderColor: theme.palette.success.main,
                            backgroundColor: 'rgba(76, 175, 80, 0.04)'
                          }
                        }}
                        variant="outlined"     
                        onClick={() => handleOpenDialog(skill)}                                       
                      >
                        Enabled
                      </Button>
                    ) : (
                      <Button
                        startIcon={<AddCircleOutlineIcon />}
                        color="secondary"
                        variant="outlined"
                        onClick={() => handleOpenDialog(skill)}
                      >
                        {isCustomApiTile ? 'Add' : 'Enable'}
                      </Button>
                    )}
                  </CardActions>
                </Card>
              </Tooltip>
            </Grid>
          );
        })}
      </Grid>

      <Menu
        anchorEl={menuAnchorEl}
        open={Boolean(menuAnchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleDisableClick}>Disable</MenuItem>
      </Menu>

      <Dialog
        open={isDisableConfirmOpen}
        onClose={() => {
          setIsDisableConfirmOpen(false);
          setSkillToDisable(null);
        }}
      >
        <DialogTitle>Disable Skill</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to disable {skillToDisable ? `"${skillToDisable}"` : 'this skill'}? All configuration will be lost once the skill is disabled.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setIsDisableConfirmOpen(false);
            setSkillToDisable(null);
          }}>Cancel</Button>
          <Button onClick={handleDisableSkill} color="error" variant="contained">
            Disable
          </Button>
        </DialogActions>
      </Dialog>

      {/* OAuth Provider Request Dialog for Regular Users */}
      <Dialog
        open={showOAuthProviderDialog}
        onClose={() => {
          setShowOAuthProviderDialog(false);
          setSelectedOAuthProvider('');
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle sx={{ display: 'flex', alignItems: 'center' }}>
          <WarningIcon sx={{ mr: 1, color: 'warning.main' }} />
          OAuth Provider Required
        </DialogTitle>
        <DialogContent>
          <DialogContentText sx={{ mb: 2 }}>
            This skill requires the <strong>{selectedOAuthProvider}</strong> OAuth provider to be enabled by an administrator.
          </DialogContentText>
          <DialogContentText>
            Please ask your admin to enable the OAuth provider for this skill. Once enabled, you'll be able to use this skill after connecting your account.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button 
            onClick={() => {
              setShowOAuthProviderDialog(false);
              setSelectedOAuthProvider('');
            }}
            variant="contained"
          >
            Got it
          </Button>
        </DialogActions>
      </Dialog>

      {renderSkillDialog()}
    </Box>
  );
};

export default Skills;
